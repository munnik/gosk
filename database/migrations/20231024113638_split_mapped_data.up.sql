ALTER TABLE "mapped_data" rename TO "mapped_data_matching_context";

CREATE TABLE "mapped_data_other_context" AS TABLE "mapped_data_matching_context" WITH NO DATA;
CREATE UNIQUE INDEX IF NOT EXISTS "mapped_data_other_context_unique_idx" ON "mapped_data_other_context"("time", "origin", "context", "path", "connector");
SELECT public.create_hypertable('mapped_data_other_context', 'time');
SELECT public.add_reorder_policy('mapped_data_other_context', 'mapped_data_other_context_unique_idx', if_not_exists => true);

CREATE VIEW "mapped_data" AS SELECT * FROM "mapped_data_matching_context" UNION ALL SELECT * FROM "mapped_data_other_context";

DROP MATERIALIZED view "transfer_local_data" CASCADE;
CREATE MATERIALIZED VIEW "transfer_local_data_other_context"
WITH (timescaledb.continuous, timescaledb.materialized_only=FALSE) AS
SELECT
    public.time_bucket(INTERVAL '5 min', "time") AS "start", 
    "origin", 
    COUNT("mapped_data_other_context"."origin") AS "count" 
FROM "mapped_data_other_context" 
GROUP BY 1, 2
WITH NO DATA;

SELECT public.add_retention_policy('transfer_local_data_other_context', INTERVAL '3 month');

SELECT public.add_continuous_aggregate_policy('transfer_local_data_other_context',
  start_offset => INTERVAL '7 day',
  end_offset => INTERVAL '1 hour',
  schedule_interval => INTERVAL '1 hour');

CREATE MATERIALIZED VIEW "transfer_local_data_mathing_context"
WITH (timescaledb.continuous, timescaledb.materialized_only=FALSE) AS
SELECT
    public.time_bucket(INTERVAL '5 min', "time") AS "start", 
    "origin", 
    COUNT("mapped_data_matching_context"."origin") AS "count" 
FROM "mapped_data_matching_context" 
GROUP BY 1, 2
WITH NO DATA;
SELECT public.add_retention_policy('transfer_local_data_mathing_context', INTERVAL '3 month');

SELECT public.add_continuous_aggregate_policy('transfer_local_data_mathing_context',
  start_offset => INTERVAL '7 day',
  end_offset => INTERVAL '1 hour',
  schedule_interval => INTERVAL '1 hour');

CREATE VIEW "transfer_local_data" AS (
  SELECT "start", "origin", sum("count") AS "count" 
  FROM (SELECT * FROM "transfer_local_data_other_context" UNION ALL SELECT * FROM "transfer_local_data_mathing_context") AS DATA 
  GROUP BY "start", "origin");


CREATE OR REPLACE VIEW "transfer_data" AS
SELECT 
    "transfer_remote_data"."origin", 
    "transfer_remote_data"."start", 
    COALESCE("transfer_local_data"."count", 0) AS "local_count", 
    "transfer_remote_data"."count" AS "remote_count"
FROM "transfer_local_data" 
RIGHT JOIN "transfer_remote_data" ON "transfer_local_data"."start" = "transfer_remote_data"."start" AND "transfer_local_data"."origin" = "transfer_remote_data"."origin"
WHERE "transfer_remote_data"."start" BETWEEN (SELECT MIN("start") FROM "transfer_local_data") AND (SELECT MAX("start") FROM "transfer_local_data");
