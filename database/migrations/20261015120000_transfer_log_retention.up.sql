-- Transfer logs are diagnostic history, not state used for retries.
-- Keep 30 days, with daily cleanup and smaller future chunks so whole-chunk
-- expiration adds at most about a day, plus the cleanup scheduling delay.
-- Existing 7-day chunks retain their width until they expire.
SELECT public.set_chunk_time_interval('transfer_log', INTERVAL '1 day');

SELECT public.add_retention_policy(
    'transfer_log',
    drop_after => INTERVAL '30 days',
    schedule_interval => INTERVAL '1 day'
);
