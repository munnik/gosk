-- Restore the previous policy and interval for future chunks.
-- Expired logs cannot be recovered by rolling back this migration.
SELECT public.remove_retention_policy('transfer_log');
SELECT public.set_chunk_time_interval('transfer_log', INTERVAL '7 days');
