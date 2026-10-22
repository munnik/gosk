-- Back to generation-only fences. That gives up what epochs protect against:
-- once they are gone the far end cannot tell one run of a vessel's sender
-- from the next, so a delivery an earlier run published and the broker still
-- holds - after a restore of that vessel's database, possibly with a higher
-- generation than anything the later run stored - could overwrite what the
-- later run delivered. Nothing stored here says whether such a delivery is
-- still on its way, so a database that has stored anything under an epoch
-- refuses the downgrade unless an operator accepts that, deliberately:
--
--   ALTER DATABASE gosk SET gosk.accept_epoch_loss = 'on';
--   -- run the down migration, then
--   ALTER DATABASE gosk RESET gosk.accept_epoch_loss;
--
-- The table is locked before it is checked, and stays locked until the
-- column is gone - the migration is one transaction - so a delivery being
-- stored under an epoch right now either commits before the check sees it,
-- or waits until the downgrade is done and is then stored by generation.
LOCK TABLE "transfer_received" IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM "transfer_received" WHERE "epoch" > 0)
       AND COALESCE(current_setting('gosk.accept_epoch_loss', true), '') <> 'on' THEN
        RAISE EXCEPTION 'transfer_received holds buckets stored under source epochs, and dropping them would let an earlier run''s delayed deliveries overwrite a later run''s; set gosk.accept_epoch_loss = on to downgrade anyway';
    END IF;
END $$;

DROP TABLE IF EXISTS "transfer_source_epoch";
ALTER TABLE "transfer_outbox" DROP COLUMN IF EXISTS "epoch";
DROP TABLE IF EXISTS "transfer_origin_epoch";
ALTER TABLE "transfer_received" DROP COLUMN IF EXISTS "epoch";
