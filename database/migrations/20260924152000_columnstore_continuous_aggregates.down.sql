DO $$
DECLARE
    aggregate TEXT;
    chunk REGCLASS;
BEGIN
    FOREACH aggregate IN ARRAY ARRAY['mapped_data_5min_hidden', 'mapped_data_1hour_hidden']
    LOOP
        IF EXISTS (
            SELECT 1 FROM timescaledb_information.continuous_aggregates
            WHERE view_name = aggregate
        ) THEN
            CALL public.remove_columnstore_policy(aggregate::REGCLASS, if_exists => true);

            -- decompress_chunk rather than CALL convert_to_rowstore: the
            -- procedure form takes transaction control, which it does not have
            -- inside the transaction golang-migrate wraps each migration in.
            FOR chunk IN
                SELECT format('%I.%I', c.chunk_schema, c.chunk_name)::REGCLASS
                FROM timescaledb_information.chunks c
                JOIN timescaledb_information.continuous_aggregates a
                    ON a.materialization_hypertable_name = c.hypertable_name
                WHERE a.view_name = aggregate AND c.is_compressed
            LOOP
                PERFORM public.decompress_chunk(chunk);
            END LOOP;

            EXECUTE format(
                'ALTER MATERIALIZED VIEW %I SET (timescaledb.enable_columnstore = false)',
                aggregate
            );
        END IF;
    END LOOP;
END $$;
