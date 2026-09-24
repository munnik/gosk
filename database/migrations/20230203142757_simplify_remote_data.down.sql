ALTER TABLE "transfer_remote_data"
RENAME TO "remote_data";

-- "end" is added nullable and only made NOT NULL after the UPDATE below
-- fills it. ADD COLUMN ... NOT NULL without a default fails outright on a
-- table that holds any rows ('column "end" of relation "remote_data"
-- contains null values'), and the UPDATE that was supposed to populate it
-- ran too late to help.
ALTER TABLE "remote_data"
ADD COLUMN "local" INTEGER NOT NULL DEFAULT -1,
ADD COLUMN "end" TIMESTAMP WITH TIME ZONE,
ADD COLUMN "count_requests" INTEGER NOT NULL DEFAULT 1,
ADD COLUMN "data_requests" INTEGER NOT NULL DEFAULT 0,
ADD COLUMN "last_data_request" TIMESTAMP WITH TIME ZONE,
ADD COLUMN "last_count_request" TIMESTAMP WITH TIME ZONE;

ALTER TABLE "remote_data"
RENAME COLUMN "count" TO "remote";

UPDATE "remote_data"
SET "end" = "start" + INTERVAL '5 min';

ALTER TABLE "remote_data"
ALTER COLUMN "end" SET NOT NULL;

-- The up migration's DROP COLUMN "end" took remote_data_pkey with it -
-- 20220809211430_alter_pk_remote_data made the key ("origin", "start",
-- "end"), and Postgres drops a constraint whose column disappears. Without
-- this the primary key stayed gone, and 20220809211430's own down
-- migration failed further along the chain with 'constraint
-- "remote_data_pkey" of relation "remote_data" does not exist'.
ALTER TABLE "remote_data"
ADD PRIMARY KEY ("origin", "start", "end");
