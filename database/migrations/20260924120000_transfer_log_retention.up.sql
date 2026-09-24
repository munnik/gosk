-- transfer_log has been a hypertable since 20230103085646 and has never
-- had a retention policy, so every count request, data request and
-- response ever sent is still in it. It is pure operational history -
-- nothing reads it to decide what to transfer - and the requester writes a
-- row per request, which on a backlog is thousands per cycle.
SELECT public.add_retention_policy('transfer_log', INTERVAL '90 days', if_not_exists => true);
