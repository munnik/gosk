ALTER TABLE "transfer_log" 
RENAME TO "transfer_log_temp";

CREATE TABLE "transfer_log" (
    "time" TIMESTAMP WITH TIME ZONE NOT NULL, 
    "origin" TEXT NOT NULL, 
    "message" JSONB NOT NULL
);

SELECT "public".create_hypertable('transfer_log', 'time');

INSERT INTO "transfer_log" (
    SELECT "time", "origin", ('{"command": "data", "uuid": "' || "uuid" || '", "period_start": "' || "start" || '"}')::JSONB
    FROM "transfer_log_temp"
);

DROP TABLE "transfer_log_temp";
