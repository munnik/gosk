-- mapped_data_matching_context and mapped_data_other_context have never been
-- compressed. Between them they are 11TB of the gosk database, and more than
-- half of that is index: on a representative chunk the btree indexes were
-- larger than the heap they point at (1981GB of index against 1586GB of heap
-- for matching_context). Converting a chunk to the columnstore drops its
-- per-chunk btrees entirely and replaces them with sparse indexes, so the
-- index half of the table disappears along with the compression of the heap.
--
-- Measured on hetzner-otap01, which runs a nightly clone of production, over a
-- six-hour slice of real mapped_data_matching_context (1170171 rows, 537MB:
-- 246MB heap + 291MB index):
--
--   segmentby=origin, orderby=time DESC                          20.96x
--   segmentby=origin, orderby=path,time DESC                     16.83x
--   segmentby=origin,context,connector, orderby=path,time DESC   16.82x
--
-- The settings below are the first of those, and the choice is not only about
-- the ratio. Putting path ahead of time in orderby sorts each batch by path,
-- which scatters any given time range across the whole chunk. Re-sending 6527
-- rows of a two-minute period - what a vessel does when it backfills - then
-- fails outright:
--
--   ERROR: tuple decompression limit exceeded by operation
--
-- because it has to decompress past max_tuples_decompressed_per_dml_transaction
-- (100000) to find the conflicting rows. Both path-ordered variants failed that
-- upsert; the time-ordered one completed in 948ms against 409ms for the same
-- upsert into the uncompressed table. Late data is normal here - the continuous
-- aggregate refresh windows are set to 180 days for exactly that reason - so an
-- ordering that cannot absorb a backfill is not usable, whatever it compresses
-- to.
--
-- Queries got faster in every shape measured, warm, against the same sample:
--
--                                        rowstore   columnstore
--   transfer reconciliation (5 min)        2.91ms       1.44ms
--   get_mapped_data (context+path, 6h)    74.07ms      11.70ms
--   continuous-aggregate refresh (1h)    139.24ms      96.22ms
--
-- The reorder policies have to go before any of this: TimescaleDB rejects a
-- reorder policy on a compressed hypertable ("cannot add reorder policy to
-- compressed hypertable") and skips compressed chunks if one is already there
-- ("Chunk will not be reordered as it has columnstore data"). orderby does the
-- job reorder was added for in 20230423230935_reorder_policy.
SELECT public.remove_reorder_policy('mapped_data_matching_context', if_exists => true);
SELECT public.remove_reorder_policy('mapped_data_other_context', if_exists => true);

ALTER TABLE "mapped_data_matching_context" SET (
    timescaledb.enable_columnstore,
    timescaledb.segmentby = 'origin',
    timescaledb.orderby = 'time DESC'
);

ALTER TABLE "mapped_data_other_context" SET (
    timescaledb.enable_columnstore,
    timescaledb.segmentby = 'origin',
    timescaledb.orderby = 'time DESC'
);

-- 60 days rather than something tighter because that is where the backfill
-- actually is. Sampling the periods transfer was retrying on 2026-08-14: 88% of
-- requests were for the current month and 6% for the month before, with a thin
-- tail reaching back a year. At 60 days effectively all routine catch-up still
-- lands in the rowstore, and only the tail pays the decompress-on-upsert cost -
-- which the settings above are chosen to survive. It leaves under 10% of either
-- table uncompressed.
CALL public.add_columnstore_policy('mapped_data_matching_context', after => INTERVAL '60 days', if_not_exists => true);
CALL public.add_columnstore_policy('mapped_data_other_context', after => INTERVAL '60 days', if_not_exists => true);
