SELECT timescaledb_experimental.alter_policies(
    'transfer_local_data_mathing_context',
    refresh_start_offset => '180 days'::interval,
    drop_after => '365 days'::interval
);

SELECT timescaledb_experimental.alter_policies(
    'transfer_local_data_other_context',
    refresh_start_offset => '180 days'::interval,
    drop_after => '365 days'::interval
);
