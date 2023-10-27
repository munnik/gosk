alter table "mapped_data" rename to "mapped_data_matching_origin";

create table "mapped_data_other_origin" as table "mapped_data_matching_origin" with no data;
CREATE UNIQUE INDEX IF NOT EXISTS "mapped_data_other_origin_unique_idx" ON "mapped_data_other_origin"("time", "origin", "context", "path", "connector");
SELECT public.create_hypertable('mapped_data_other_origin', 'time');
SELECT public.add_reorder_policy('mapped_data_other_origin', 'mapped_data_other_origin_unique_idx', if_not_exists => true);

create view mapped_data as select * from mapped_data_matching_origin union all select * from mapped_data_other_origin;

drop MATERIALIZED view "transfer_local_data" cascade;
CREATE MATERIALIZED VIEW "transfer_local_data_1"
WITH (timescaledb.continuous, timescaledb.materialized_only=FALSE) AS
SELECT
    public.time_bucket(INTERVAL '5 min', "time") AS "start", 
    "origin", 
    COUNT("mapped_data_other_origin"."origin") AS "count" 
FROM "mapped_data_other_origin" 
GROUP BY 1, 2
WITH NO DATA;

SELECT public.add_retention_policy('transfer_local_data_1', INTERVAL '3 month');

SELECT public.add_continuous_aggregate_policy('transfer_local_data_1',
  start_offset => INTERVAL '7 day',
  end_offset => INTERVAL '1 hour',
  schedule_interval => INTERVAL '1 hour');

CREATE MATERIALIZED VIEW "transfer_local_data_2"
WITH (timescaledb.continuous, timescaledb.materialized_only=FALSE) AS
SELECT
    public.time_bucket(INTERVAL '5 min', "time") AS "start", 
    "origin", 
    COUNT("mapped_data_matching_origin"."origin") AS "count" 
FROM "mapped_data_matching_origin" 
GROUP BY 1, 2
WITH NO DATA;
SELECT public.add_retention_policy('transfer_local_data_2', INTERVAL '3 month');

SELECT public.add_continuous_aggregate_policy('transfer_local_data_2',
  start_offset => INTERVAL '7 day',
  end_offset => INTERVAL '1 hour',
  schedule_interval => INTERVAL '1 hour');

create view transfer_local_data as (
  select start, origin, sum(count) as count 
  from (select * from transfer_local_data_1 union all select * from transfer_local_data_2) as data 
  group by start, origin);


CREATE OR REPLACE VIEW "transfer_data" AS
SELECT 
    "transfer_remote_data"."origin", 
    "transfer_remote_data"."start", 
    COALESCE("transfer_local_data"."count", 0) AS "local_count", 
    "transfer_remote_data"."count" AS "remote_count"
FROM "transfer_local_data" 
RIGHT JOIN "transfer_remote_data" ON "transfer_local_data"."start" = "transfer_remote_data"."start" AND "transfer_local_data"."origin" = "transfer_remote_data"."origin"
WHERE "transfer_remote_data"."start" BETWEEN (SELECT MIN("start") FROM "transfer_local_data") AND (SELECT MAX("start") FROM "transfer_local_data");
