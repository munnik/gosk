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

-- ->> with a single-quoted key, not -> with a double-quoted one. Double
-- quotes make "uuid" an identifier, so this read as a reference to a
-- column of transfer_log_temp that does not exist and the downgrade failed
-- with 'column "uuid" does not exist'. -> would also have yielded jsonb
-- where these columns want uuid and timestamptz.
--
-- Both columns are NOT NULL, and the message shape they are read out of
-- only exists for rows the up migration wrote or that were logged after
-- it, so a row without them falls back rather than failing the migration.
INSERT INTO "transfer_log" (
    SELECT
        "origin",
        "time",
        COALESCE(("message" ->> 'uuid')::UUID, "public".uuid_nil()),
        COALESCE(("message" ->> 'period_start')::TIMESTAMPTZ, "time"),
        COALESCE(("message" ->> 'period_start')::TIMESTAMPTZ, "time") + '5 min'::interval,
        -1,
        -1
    FROM "transfer_log_temp"
);

DROP TABLE "transfer_log_temp";
