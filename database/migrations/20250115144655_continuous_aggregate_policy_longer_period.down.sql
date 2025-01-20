SELECT timescaledb_experimental.alter_policies(
    'transfer_local_data_mathing_context',
    refresh_start_offset => '7 days'::interval
    drop_after => '3 month'::interval
);

SELECT timescaledb_experimental.alter_policies(
    'transfer_local_data_other_context',
    refresh_start_offset => '7 days'::interval
    drop_after => '3 month'::interval
);
