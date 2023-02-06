ALTER TABLE "transfer_remote_data"
RENAME TO "remote_data";

ALTER TABLE "remote_data" 
ADD COLUMN "local" INTEGER NOT NULL DEFAULT -1;

ALTER TABLE "remote_data" 
ADD COLUMN "end" TIMESTAMP WITH TIME ZONE NOT NULL;

UPDATE "remote_data"
SET "end" = "start" + INTERVAL '5 min';
