CALL public.remove_columnstore_policy('mapped_data_matching_context', if_exists => true);
CALL public.remove_columnstore_policy('mapped_data_other_context', if_exists => true);

-- decompress_chunk rather than CALL convert_to_rowstore: the procedure form
-- takes transaction control, which it does not have inside the transaction
-- golang-migrate wraps each migration in. This rewrites every compressed chunk
-- back into the rowstore and will take a long time on a database of this size.
DO $$
DECLARE
    chunk REGCLASS;
BEGIN
    FOR chunk IN
        SELECT format('%I.%I', chunk_schema, chunk_name)::REGCLASS
        FROM timescaledb_information.chunks
        WHERE hypertable_name IN ('mapped_data_matching_context', 'mapped_data_other_context')
            AND is_compressed
    LOOP
        PERFORM public.decompress_chunk(chunk);
    END LOOP;
END $$;

ALTER TABLE "mapped_data_matching_context" SET (timescaledb.enable_columnstore = false);
ALTER TABLE "mapped_data_other_context" SET (timescaledb.enable_columnstore = false);

SELECT public.add_reorder_policy('mapped_data_matching_context', 'mapped_data_unique_idx', if_not_exists => true);
SELECT public.add_reorder_policy('mapped_data_other_context', 'mapped_data_other_context_unique_idx', if_not_exists => true);
