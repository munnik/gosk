-- The mapped_data_5min_hidden and mapped_data_1hour_hidden continuous
-- aggregates carry the same index-heavy shape as the hypertables they are
-- built from - 405GB of index against 283GB of heap for the 5min one, 699GB in
-- total - and have never been compressed either.
--
-- These two aggregates are created by the nix repo's per-host migrations and
-- exist only on the servers, not on a vessel node, so everything here is
-- guarded on the view actually being there. On a node this migration is a
-- no-op.
--
-- 200 days, not the 60 used for the hypertables, because of the refresh
-- windows. Both policies were widened to refresh_start_offset => 180 days in
-- the nix repo's 20250114153630_continuous_aggregate_policy_longer_period, so
-- anything compressed inside that window would be decompressed and recompressed
-- on every refresh run - and those runs are already slow enough to have needed
-- a max_runtime cap in 20260915170000_cap_mapped_data_aggregate_refresh_runtime.
-- 200 days keeps the compression boundary clear of the refresh boundary.
DO $$
DECLARE
    aggregate TEXT;
BEGIN
    FOREACH aggregate IN ARRAY ARRAY['mapped_data_5min_hidden', 'mapped_data_1hour_hidden']
    LOOP
        IF EXISTS (
            SELECT 1 FROM timescaledb_information.continuous_aggregates
            WHERE view_name = aggregate
        ) THEN
            EXECUTE format(
                'ALTER MATERIALIZED VIEW %I SET ('
                || 'timescaledb.enable_columnstore = true, '
                || 'timescaledb.segmentby = ''origin'', '
                || 'timescaledb.orderby = ''time DESC'')',
                aggregate
            );
            CALL public.add_columnstore_policy(aggregate::REGCLASS, after => INTERVAL '200 days', if_not_exists => true);
            RAISE NOTICE 'Enabled columnstore on %', aggregate;
        ELSE
            RAISE NOTICE 'Skipping %, not present on this host', aggregate;
        END IF;
    END LOOP;
END $$;
