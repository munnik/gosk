CREATE MATERIALIZED VIEW "transfer_local_data"
WITH (timescaledb.continuous, timescaledb.materialized_only=FALSE) AS
SELECT
    public.time_bucket(INTERVAL '5 min', "time") AS "start", 
    "origin", 
    COUNT("mapped_data"."origin") AS "count" 
FROM "mapped_data" 
GROUP BY 1, 2
WITH NO DATA;

SELECT public.add_retention_policy('transfer_local_data', INTERVAL '3 month');

SELECT public.add_continuous_aggregate_policy('transfer_local_data',
  start_offset => INTERVAL '3 month',
  end_offset => INTERVAL '1 hour',
  schedule_interval => INTERVAL '1 hour');
