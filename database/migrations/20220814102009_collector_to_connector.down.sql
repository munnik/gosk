ALTER TABLE "mapped_data"
RENAME COLUMN "connector" TO "collector";
ALTER TABLE "raw_data"
RENAME COLUMN "connector" TO "collector";
