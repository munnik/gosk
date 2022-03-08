ALTER TABLE "raw_data"
ADD COLUMN "type" TEXT NOT NULL DEFAULT 'unknown';

ALTER TABLE "mapped_data"
ADD COLUMN "type" TEXT NOT NULL DEFAULT 'unknown';
