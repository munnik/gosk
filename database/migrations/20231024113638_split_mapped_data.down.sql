-- Reverses 20231024113638_split_mapped_data.up.sql.
--
-- This used to name mapped_data_matching_origin and
-- mapped_data_other_origin, tables the up migration never creates - it
-- splits mapped_data on context, not on origin. The very first statement
-- therefore failed, golang-migrate marked the schema dirty, and every
-- later attempt to migrate refused with "Dirty database version
-- 20231024113638. Fix and force version." That is what database's test
-- suite hits in its AfterEach, which downgrades between specs.
--
-- The reversal was also incomplete: it left behind the two continuous
-- aggregates and the two views the up migration introduced, and never put
-- back the single transfer_local_data aggregate they replaced, so even
-- with the names corrected a downgraded schema did not match what the
-- previous migration left.

-- transfer_data reads transfer_local_data, so it goes first.
DROP VIEW "transfer_data";
DROP VIEW "transfer_local_data";
DROP MATERIALIZED VIEW "transfer_local_data_mathing_context";
DROP MATERIALIZED VIEW "transfer_local_data_other_context";

DROP VIEW "mapped_data";
ALTER TABLE "mapped_data_matching_context" RENAME TO "mapped_data";
--  mapped_data_other_context is not dropped because that would result in data loss and inserting it back into mapped data would be too slow

-- transfer_local_data as 20230203140028 created it, with the policy
-- 20230517122705 then changed it to.
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
  start_offset => INTERVAL '7 day',
  end_offset => INTERVAL '1 hour',
  schedule_interval => INTERVAL '1 hour');

-- transfer_data as 20230307145243 left it.
CREATE VIEW "transfer_data" AS
SELECT
    "transfer_remote_data"."origin",
    "transfer_remote_data"."start",
    "transfer_local_data"."count" AS "local_count",
    "transfer_remote_data"."count" AS "remote_count"
FROM
    "transfer_remote_data"
INNER JOIN
    "transfer_local_data"
    ON "transfer_local_data"."start" = "transfer_remote_data"."start"
    AND "transfer_local_data"."origin" = "transfer_remote_data"."origin"
WHERE
    "transfer_remote_data"."start" BETWEEN (SELECT MIN("start") FROM "transfer_local_data") AND (SELECT MAX("start") FROM "transfer_local_data")
UNION
SELECT
    "transfer_remote_data"."origin",
    "transfer_remote_data"."start",
    0 AS "local_count",
    "transfer_remote_data"."count" AS "remote_count"
FROM
    "transfer_remote_data"
WHERE
    "transfer_remote_data"."start" BETWEEN (SELECT MIN("start") FROM "transfer_local_data") AND (SELECT MAX("start") FROM "transfer_local_data")
    AND NOT EXISTS (
        SELECT
            1
        FROM
            "transfer_local_data"
        WHERE
            "transfer_local_data"."start" = "transfer_remote_data"."start"
            AND "transfer_local_data"."origin" = "transfer_remote_data"."origin"
    )
;
