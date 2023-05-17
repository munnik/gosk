SELECT public.remove_continuous_aggregate_policy('transfer_local_data');

SELECT public.add_continuous_aggregate_policy('transfer_local_data',
  start_offset => INTERVAL '7 day',
  end_offset => INTERVAL '1 hour',
  schedule_interval => INTERVAL '1 hour');
