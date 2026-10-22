-- Back to the TimescaleDB default both hypertables were created with. Only
-- chunks created after this run are affected; 1-day chunks already made keep
-- their width.
SELECT public.set_chunk_time_interval('mapped_data_matching_context', INTERVAL '7 days');
SELECT public.set_chunk_time_interval('mapped_data_other_context', INTERVAL '7 days');
