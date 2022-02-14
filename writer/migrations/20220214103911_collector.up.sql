ALTER TABLE "raw_data"
RENAME COLUMN "key" TO "collector";
ALTER TABLE "mapped_data"
RENAME COLUMN "key" TO "collector";
