-- The count-based transfer protocol (gosk transfer request / respond) is
-- replaced by the outbox of 20261017120000, so everything it kept goes:
--
-- - transfer_remote_data: the vessels' reported row counts per 5-minute
--   period, O(origins x periods) and never expired.
-- - transfer_local_data: a view over two continuous aggregates counting the
--   local rows per period, refreshed hourly over the last 180 days.
-- - transfer_data: the comparison of the two.
-- - transfer_log: every request and response ever sent.
--
-- The continuous aggregates must go before their retention and refresh
-- policies would otherwise fail on a missing view, and the views that depend
-- on them go first of all.
DROP VIEW IF EXISTS "transfer_data";
DROP VIEW IF EXISTS "transfer_local_data";
DROP MATERIALIZED VIEW IF EXISTS "transfer_local_data_other_context";
DROP MATERIALIZED VIEW IF EXISTS "transfer_local_data_mathing_context";
DROP TABLE IF EXISTS "transfer_remote_data";
DROP TABLE IF EXISTS "transfer_log";
