ALTER TABLE "mapped_data"
RENAME COLUMN "collector" TO "key";
ALTER TABLE "raw_data"
RENAME COLUMN "collector" TO "key";
