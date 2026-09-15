DO $$
DECLARE
    job_id integer;
BEGIN
    FOR job_id IN
        SELECT j.job_id
        FROM timescaledb_information.jobs j
        WHERE j.proc_name = 'policy_refresh_continuous_aggregate'
            AND j.hypertable_name IN ('transfer_local_data_mathing_context', 'transfer_local_data_other_context')
    LOOP
        PERFORM alter_job(job_id, max_runtime => INTERVAL '0');
    END LOOP;
END $$;
