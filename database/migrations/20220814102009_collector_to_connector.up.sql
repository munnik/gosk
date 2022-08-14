ALTER TABLE "raw_data"
RENAME COLUMN "collector" TO "connector";
ALTER TABLE "mapped_data"
RENAME COLUMN "collector" TO "connector";
