ALTER TABLE "transfer_remote_data"
RENAME TO "remote_data";

ALTER TABLE "remote_data" 
ADD COLUMN "local" INTEGER NOT NULL DEFAULT -1,
ADD COLUMN "end" TIMESTAMP WITH TIME ZONE NOT NULL,
ADD COLUMN "count_requests" INTEGER NOT NULL DEFAULT 1,
ADD COLUMN "data_requests" INTEGER NOT NULL DEFAULT 0,
ADD COLUMN "last_data_request" TIMESTAMP WITH TIME ZONE,
ADD COLUMN "last_count_request" TIMESTAMP WITH TIME ZONE;

ALTER TABLE "remote_data" 
RENAME COLUMN "count" TO "remote";

UPDATE "remote_data"
SET "end" = "start" + INTERVAL '5 min';
