-- transfer_log had no retention policy at all; on the-production-server it had grown
-- to N GB, larger than mapped_data_matching_context and most of
-- mapped_data_other_context put together, almost all of it counts_per_uuid
-- maps logged in full. Chunks shrink to one day so the policy drops a day at
-- a time rather than a 7-day chunk at once.
--
-- if_not_exists on both: the timescale-columnstore branch carried the same
-- two statements under an earlier number for a while, and a database that
-- ran those first would otherwise fail here on the policy already being
-- there.
SELECT public.set_chunk_time_interval('transfer_log', INTERVAL '1 day');

SELECT public.add_retention_policy(
    'transfer_log',
    drop_after => INTERVAL '30 days',
    schedule_interval => INTERVAL '1 day',
    if_not_exists => true
);
