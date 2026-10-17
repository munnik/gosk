-- Only the policy and the chunk interval come back. The data the policy has
-- already dropped is gone.
SELECT public.remove_retention_policy('transfer_log', if_exists => true);
SELECT public.set_chunk_time_interval('transfer_log', INTERVAL '7 days');
