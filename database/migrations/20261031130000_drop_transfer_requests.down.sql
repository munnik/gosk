-- Recreates the structures as they were after 20261015120000, empty. The
-- data they held cannot be brought back, and nothing would want it: it only
-- ever described the state of a protocol that no longer runs.
CREATE TABLE "transfer_log" (
    "origin"  TEXT NOT NULL,
    "time"    TIMESTAMP WITH TIME ZONE NOT NULL,
    "message" JSONB
);
SELECT "public".create_hypertable('transfer_log', 'time', chunk_time_interval => INTERVAL '1 day');
SELECT public.add_retention_policy('transfer_log', drop_after => INTERVAL '30 days', schedule_interval => INTERVAL '1 day', if_not_exists => true);

CREATE TABLE "transfer_remote_data" (
    "start"  TIMESTAMP WITH TIME ZONE NOT NULL,
    "origin" TEXT NOT NULL,
    "count"  INTEGER NOT NULL DEFAULT 0
);
CREATE UNIQUE INDEX "idx_transfer_remote_data_start_origin" ON "transfer_remote_data" ("start", "origin");

CREATE MATERIALIZED VIEW "transfer_local_data_other_context"
WITH (timescaledb.continuous, timescaledb.materialized_only=FALSE) AS
SELECT
    public.time_bucket(INTERVAL '5 min', "time") AS "start",
    "origin",
    COUNT("mapped_data_other_context"."origin") AS "count"
FROM "mapped_data_other_context"
GROUP BY 1, 2
WITH NO DATA;
SELECT public.add_retention_policy('transfer_local_data_other_context', INTERVAL '365 days');
SELECT public.add_continuous_aggregate_policy('transfer_local_data_other_context',
  start_offset => INTERVAL '180 day',
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
SELECT public.add_retention_policy('transfer_local_data_mathing_context', INTERVAL '365 days');
SELECT public.add_continuous_aggregate_policy('transfer_local_data_mathing_context',
  start_offset => INTERVAL '180 day',
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
    "transfer_local_data"."count" AS "local_count",
    "transfer_remote_data"."count" AS "remote_count"
FROM "transfer_remote_data"
INNER JOIN "transfer_local_data"
    ON "transfer_local_data"."start" = "transfer_remote_data"."start"
    AND "transfer_local_data"."origin" = "transfer_remote_data"."origin"
WHERE "transfer_remote_data"."start" BETWEEN (SELECT MIN("start") FROM "transfer_local_data") AND (SELECT MAX("start") FROM "transfer_local_data")
UNION
SELECT
    "transfer_remote_data"."origin",
    "transfer_remote_data"."start",
    0 AS "local_count",
    "transfer_remote_data"."count" AS "remote_count"
FROM "transfer_remote_data"
WHERE "transfer_remote_data"."start" BETWEEN (SELECT MIN("start") FROM "transfer_local_data") AND (SELECT MAX("start") FROM "transfer_local_data")
    AND NOT EXISTS (
        SELECT 1 FROM "transfer_local_data"
        WHERE "transfer_local_data"."start" = "transfer_remote_data"."start"
            AND "transfer_local_data"."origin" = "transfer_remote_data"."origin"
    );
