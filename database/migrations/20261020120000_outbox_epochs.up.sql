-- Source epochs: a fencing token per run of a vessel's sender.
--
-- The far end keeps, per (origin, bucket), the newest generation it has
-- stored (20261017120300_transfer_received), and refuses anything older, so
-- that a delayed delivery of an old generation cannot overwrite a newer one.
-- That holds only while a vessel's generations keep rising, and they do not
-- when its database is restored from an earlier snapshot: the outbox and its
-- sequence go back, buckets pending at the snapshot are sent again under
-- generations the far end has seen exceeded and are refused for good, and
-- deliveries the previous incarnation published but the far end never
-- received - held by the broker - can carry generations above anything the
-- far end has seen, so no number the restored database could pick from what
-- the far end holds is sure to beat them.
--
-- So every sender, when it starts, reserves an epoch from the far end
-- (database.ReserveOutboxEpoch, through the digest responder) before it
-- sends anything, and every delivery carries it. The far end hands out each
-- origin a number above any it has handed out or stored for that origin
-- before, and orders a bucket's deliveries by (epoch, generation). A run of
-- a sender is thereby ordered after every earlier run, whatever its database
-- says: a restore always means a new run, because the gosk units stop with
-- the database's container and nothing can roll its dataset back while it is
-- mounted. Everything an earlier run sent, seen or not, is older than
-- anything a later one sends; within a run, generations rise as before.
--
-- Progress belongs to a run. Each outbox bucket on the sending end records
-- the epoch its sent and acknowledged watermarks were made under, and a run
-- that reads a bucket last progressed under another epoch starts it over:
-- nothing sent or acknowledged, the whole bucket again. Otherwise a new run
-- would send only the rows after an earlier run's acknowledgement, and a far
-- end that had not yet stored the earlier run's delivery - a copy, whose
-- deliveries can be ordered either way - would store the later one first
-- and then refuse the earlier one for good. An acknowledgement applies only
-- under the epoch the bucket is on, so one from an earlier run, delayed,
-- cannot settle rows this run has not sent.
--
-- Senders older than this send no epoch, which reads as 0, and are compared
-- by generation alone, exactly as before - but only in buckets nothing with
-- an epoch has been stored in. Once one has, an epoch-less delivery is an
-- earlier run's by definition (senders are upgraded after their far end and
-- not rolled back), and is refused.
--
-- transfer_origin_epoch records, on the far end, the highest epoch reserved
-- per origin; transfer_source_epoch records, on the sending end, the last
-- epoch this database's sender reserved. A reservation is above both and
-- above anything stored under the origin, so one end losing its history -
-- a restored source, or a far end remade from an older copy, as
-- the-test-server's nightly clone is - cannot make epochs go back. Both ends
-- losing it can, and not only at once: a far end remade while a vessel is
-- offline forgets that vessel's epoch 7, and the vessel restored later to
-- a snapshot from its epoch 3 then reserves 4. A delivery of epoch 7 still
-- held by a broker would then win over everything the restored run sends,
-- and a copy that kept epoch 7 would refuse it. On the-test-server the far end
-- is remade every night by design, so a vessel in its trial that is
-- restored must not be restored to a snapshot older than its last run
-- without clearing the copy's and broker's state for it too; production's
-- far end is not remade. This design does not close it. What would is an
-- allocator whose state survives both a vessel's restore and the far end
-- being remade - kept somewhere neither touches - or a clock both ends
-- trust, which the vessels' clocks are not; the fleet has neither today.
ALTER TABLE "transfer_received" ADD COLUMN "epoch" BIGINT NOT NULL DEFAULT 0;

CREATE TABLE "transfer_origin_epoch" (
    "origin" TEXT PRIMARY KEY,
    "epoch"  BIGINT NOT NULL
);

ALTER TABLE "transfer_outbox" ADD COLUMN "epoch" BIGINT NOT NULL DEFAULT 0;

CREATE TABLE "transfer_source_epoch" (
    "one"   BOOLEAN PRIMARY KEY DEFAULT TRUE CHECK ("one"),
    "epoch" BIGINT NOT NULL
);
