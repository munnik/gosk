ALTER TABLE "remote_data" 
ADD COLUMN "count_requests" INTEGER NOT NULL DEFAULT 1,
ADD COLUMN "data_requests" INTEGER NOT NULL DEFAULT 0,
ADD COLUMN "last_data_request" TIMESTAMP WITH TIME ZONE,
ADD COLUMN "last_count_request" TIMESTAMP WITH TIME ZONE;

CREATE TABLE "transfer_log" (
    "origin" TEXT NOT NULL, 
    "time" TIMESTAMP WITH TIME ZONE NOT NULL, 
    "uuid" UUID NOT NULL DEFAULT "public".uuid_nil(),
    "start" TIMESTAMP WITH TIME ZONE NOT NULL, 
    "end" TIMESTAMP WITH TIME ZONE NOT NULL, 
    "local" INTEGER NOT NULL DEFAULT -1, 
    "remote" INTEGER NOT NULL DEFAULT 0
);

SELECT "public".create_hypertable('transfer_log', 'time');

ALTER TABLE "mapped_data" 
ADD COLUMN "transfer_uuid" UUID;
