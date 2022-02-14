CREATE TABLE "raw_data" (
    "_time" TIMESTAMP WITH TIME ZONE NOT NULL,
    "_key" CHARACTER VARYING [] NOT NULL,
    "_value" BYTEA NOT NULL
);
SELECT create_hypertable('raw_data', '_time');