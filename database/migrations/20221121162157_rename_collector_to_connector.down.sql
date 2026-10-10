-- This used to read RENAME COLUMN "collector" TO "collector" on both
-- tables - a no-op name that does not exist at this point, since the up
-- migration renamed it to "connector". Postgres rejected the first
-- statement with 'column "collector" does not exist' and the downgrade
-- stopped here with the schema dirty.
ALTER TABLE "raw_data"
RENAME COLUMN "connector" TO "collector";
ALTER TABLE "mapped_data"
RENAME COLUMN "connector" TO "collector";
