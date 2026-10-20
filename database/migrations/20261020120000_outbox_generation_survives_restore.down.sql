-- Back to the bare sequence. The sequence has been moved up to every value
-- given out, so what it hands out next is still newer than any generation
-- already stored.
ALTER TABLE "transfer_outbox" ALTER COLUMN "generation" SET DEFAULT nextval('transfer_outbox_generation_seq');
DROP FUNCTION IF EXISTS "transfer_outbox_next_generation"();
