-- Why is data missing? Run this on the central server. It separates the
-- two failures that look identical from the outside: the count protocol
-- not NOTICING that data is missing, and it noticing but not managing to
-- fetch it.
--
--     psql "$GOSK_URL" -f diagnose_transfer.sql

\echo '== 1. is the continuous aggregate the whole protocol rests on actually refreshing? =='
-- transfer_local_data is what "how much do I have" is read from. If its
-- refresh job is failing or overrunning, local_count is understated and
-- every period looks incomplete - or it is stale and they look complete
-- when they are not. 20250115144655 widened the refresh window to 180
-- days while leaving the schedule at one hour, which is what motivated
-- 20260915160000 capping max_runtime.
SELECT j.hypertable_name,
       j.schedule_interval,
       j.max_runtime,
       s.last_run_status,
       s.last_successful_finish,
       s.total_runs,
       s.total_failures,
       s.last_run_duration
FROM timescaledb_information.jobs j
LEFT JOIN timescaledb_information.job_stats s USING (job_id)
WHERE j.proc_name = 'policy_refresh_continuous_aggregate'
ORDER BY 1;

\echo ''
\echo '== 2. how far behind is it? =='
-- A materialisation watermark well behind now means every period after it
-- is being counted from the real-time part of the view, or not at all.
SELECT view_name,
       public.to_timestamp(
           public.cagg_watermark(format('%I.%I', materialization_hypertable_schema, materialization_hypertable_name)::regclass::oid) / 1000000
       ) AS materialized_up_to
FROM timescaledb_information.continuous_aggregates
WHERE view_name LIKE 'transfer_local_data%';

\echo ''
\echo '== 3. how much does the protocol think is missing, per origin? =='
SELECT origin,
       count(*)                                        AS periods_known,
       count(*) FILTER (WHERE local_count < remote_count)  AS periods_short,
       sum(remote_count - local_count) FILTER (WHERE local_count < remote_count) AS rows_short,
       min(start) FILTER (WHERE local_count < remote_count) AS oldest_short,
       max(start) FILTER (WHERE local_count < remote_count) AS newest_short
FROM transfer_data
GROUP BY 1 ORDER BY 1;

\echo ''
\echo '== 4. how far back can it see at all? =='
-- Everything older than this is outside the protocol's reach, whatever it
-- reports: transfer_data is bounded by these, and the aggregates are
-- dropped after 365 days.
SELECT min(start) AS oldest_period_known, max(start) AS newest_period_known
FROM transfer_local_data;

\echo ''
\echo '== 5. is it asking, and is anything coming back? =='
-- A request logged with no data arriving afterwards points at the repair
-- path being lost rather than the gap not being seen. Before the QoS 1
-- change both the request and the data it asked for went out at QoS 0.
SELECT date_trunc('hour', time) AS hour,
       count(*) FILTER (WHERE message->>'command' = 'count') AS count_msgs,
       count(*) FILTER (WHERE message->>'command' = 'data')  AS data_msgs
FROM transfer_log
WHERE time > now() - interval '24 hours'
GROUP BY 1 ORDER BY 1 DESC LIMIT 24;
