ALTER TABLE "raw_data"
RENAME COLUMN "uuid" TO "raw_uuid";

ALTER TABLE "mapped_data"
RENAME COLUMN "uuid" TO "raw_uuid";
