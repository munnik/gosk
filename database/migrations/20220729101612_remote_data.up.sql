CREATE TABLE "remote_data" (
    "origin" TEXT NOT NULL DEFAULT 'unknown', 
    "start" TIMESTAMP WITH TIME ZONE NOT NULL, 
    "end" TIMESTAMP WITH TIME ZONE NOT NULL, 
    "local" INTEGER NOT NULL DEFAULT -1, 
    "remote" INTEGER NOT NULL DEFAULT 0
);
SELECT create_hypertable('remote_data', 'start');