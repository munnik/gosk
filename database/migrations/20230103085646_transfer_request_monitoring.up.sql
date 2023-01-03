ALTER TABLE "remote_data" 
ADD COLUMN "count_requests" INTEGER NOT NULL DEFAULT 0,
ADD COLUMN "last_request" TIMESTAMP WITH TIME ZONE NOT NULL;

CREATE TABLE "transfer_log" (
    "origin" TEXT NOT NULL, 
    "time" TIMESTAMP WITH TIME ZONE NOT NULL, 
    "uuid" UUID NOT NULL DEFAULT uuid_nil(),
    "start" TIMESTAMP WITH TIME ZONE NOT NULL, 
    "end" TIMESTAMP WITH TIME ZONE NOT NULL, 
    "local" INTEGER NOT NULL DEFAULT -1, 
    "remote" INTEGER NOT NULL DEFAULT 0
);

-- alter table mapped_data with uuid and maybe transfer time

SELECT public.create_hypertable('transfer_log', 'time');