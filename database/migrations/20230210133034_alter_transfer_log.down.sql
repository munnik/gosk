ALTER TABLE "transfer_log" 
RENAME TO "transfer_log_temp";

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

INSERT INTO "transfer_log" (
    SELECT 
        "origin", 
        "time",
        "message"->"uuid",
        "message"->"period_start",
        "message"->"period_start" + '5 min'::interval,
        -1,
        -1
    FROM "transfer_log_temp"
);

DROP TABLE "transfer_log_temp";
