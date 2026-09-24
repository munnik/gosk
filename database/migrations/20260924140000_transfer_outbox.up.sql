-- The outbox half of the delivery scheme described in section 4.2 of
-- TRANSFER_REVIEW.md: instead of asking a vessel to count what it has and
-- comparing, the vessel keeps a list of what it has not yet had
-- acknowledged, and works through it.
--
-- One row per (origin, uuid), not per mapped row. A single source message
-- becomes several mapped rows - one per path - and they all carry its
-- uuid, so the uuid is the natural unit to ship and to acknowledge. It is
-- also the unit SelectCountPerUuid already counts in, which keeps the two
-- schemes talking about the same thing while they run side by side.
--
-- There is no sequence column and no timestamp column here. Every uuid is
-- now a version 7 uuid, so ordering by uuid is ordering by time, and the
-- oldest unacknowledged work sorts first without anything else being
-- stored. See section 6.
CREATE TABLE "transfer_outbox" (
    "origin" TEXT NOT NULL,
    "uuid"   UUID NOT NULL,
    PRIMARY KEY ("origin", "uuid")
);

-- The shipper asks for the oldest pending work across all origins, so the
-- primary key's leading "origin" is the wrong order for it. Ordering by
-- uuid alone is ordering by time.
CREATE INDEX "transfer_outbox_uuid_idx" ON "transfer_outbox" ("uuid");

COMMENT ON TABLE "transfer_outbox" IS
    'Source messages written locally but not yet acknowledged by the far end. A row is pending if and only if it is here; there is no watermark to advance.';
