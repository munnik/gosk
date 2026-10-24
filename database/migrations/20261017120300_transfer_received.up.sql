-- The far end's side of the outbox: which generation of each bucket it has
-- stored. One row per (origin, bucket), written in the same transaction as
-- the delivery's rows (see StoreDelivery in database/outbox.go).
--
-- A late row on the vessel resets its bucket to a new generation and the
-- whole bucket is sent again. A delivery of the old generation can still
-- be on its way - held by the broker for a receiver that was down, or
-- behind the new one in paho's concurrent handlers - and storing it after
-- the new one would put the old contents over the new for every row the
-- two share. The vessel is done with the old generation by then and will
-- never send the new one again, so nothing would ever put it right. Hence a
-- delivery is stored only if its generation is at least the one recorded
-- here, and the record lives in the database rather than in the receiver's
-- memory so that neither a restart nor a second receiver on the same
-- database forgets it.
--
-- The rows are kept for good: nothing bounds how long a delivery of an
-- old generation can be on its way - a broker holds what a receiver that
-- was down has not taken - and a vessel's clock cannot be trusted to date
-- it, so no age makes a row safe to forget. One row per bucket that was
-- ever delivered, about a hundred bytes each, is cheap. received is when
-- the row was last written, for looking into things.
CREATE TABLE "transfer_received" (
    "origin"     TEXT NOT NULL,
    "bucket"     TIMESTAMP WITH TIME ZONE NOT NULL,
    "generation" BIGINT NOT NULL,
    "received"   TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT now(),
    PRIMARY KEY ("origin", "bucket")
);

COMMENT ON TABLE "transfer_received" IS
    'The newest generation of each outbox bucket stored from a delivery; a delivery of an older generation is not stored. Kept for good.';
