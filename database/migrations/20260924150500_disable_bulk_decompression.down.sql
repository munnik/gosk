-- Dynamic SQL for the same reason as the up migration: ALTER DATABASE takes
-- a literal name, and the database is not called gosk everywhere.
DO $$
BEGIN
    EXECUTE format(
        'ALTER DATABASE %I RESET timescaledb.enable_bulk_decompression',
        current_database()
    );
END $$;
