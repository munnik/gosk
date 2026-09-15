-- transfer_local_data_{mathing,other}_context's refresh policy has no
-- max_runtime (TimescaleDB's default is unlimited), and its refresh window
-- was widened to 180 days by 20250115144655_continuous_aggregate_policy_longer_period
-- while keeping the same 1-hour schedule_interval. A refresh over that much
-- data can now take well over an hour, and with no timeout to cancel an
-- overrunning one, the next scheduled run starts on top of it instead of
-- replacing it - they pile up and compete for the same locks/I/O
-- indefinitely. Cap it comfortably under the schedule interval so an
-- overrunning refresh gets cancelled instead.
DO $$
DECLARE
    job_id integer;
BEGIN
    FOR job_id IN
        SELECT j.job_id
        FROM timescaledb_information.jobs j
        WHERE j.proc_name = 'policy_refresh_continuous_aggregate'
            AND j.hypertable_name IN ('transfer_local_data_mathing_context', 'transfer_local_data_other_context')
    LOOP
        PERFORM alter_job(job_id, max_runtime => INTERVAL '45 minutes');
    END LOOP;
END $$;
