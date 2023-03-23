ALTER TABLE "raw_data"
RENAME COLUMN "raw_uuid" TO "uuid";

ALTER TABLE "mapped_data"
RENAME COLUMN "raw_uuid" TO "uuid";
