-- enable timescaledb extension
CREATE extension IF NOT EXISTS timescaledb CASCADE;
--
--
-- create a table for vessel static data
DROP TABLE IF EXISTS "static_data";
CREATE TABLE "static_data"(
    "context" TEXT NOT NULL PRIMARY KEY,
    "name" TEXT,
    "mmsi" TEXT
);
--
--
-- create a table for raw timeseries data
DROP TABLE IF EXISTS "raw_data";
CREATE TABLE "raw_data" (
    "time" TIMESTAMPTZ NOT NULL,
    "key" TEXT [] NOT NULL,
    "value" BYTEA NOT NULL -- base64 encoded binary data
);
SELECT create_hypertable('raw_data', 'time');
--
--
-- create a table for mapped timeseries data
DROP TABLE IF EXISTS "key_value_data";
CREATE TABLE "key_value_data" (
    "time" TIMESTAMPTZ NOT NULL,
    "context" TEXT NULL,
    "key" TEXT [] NOT NULL,
    "path" TEXT NOT NULL,
    "value" JSON NOT NULL
);
CREATE INDEX "idx_path" ON "key_value_data"("path");
SELECT create_hypertable('key_value_data', 'time');
--
--
-- function and trigger to create or update vessel name in static_data
CREATE OR REPLACE FUNCTION "update_name"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "static_data" ("context", "name")
VALUES (NEW."context", NEW."value_text") ON CONFLICT("context") DO
UPDATE
SET "name" = NEW."value_text";
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';
DROP TRIGGER IF EXISTS "update_name_trigger" ON "key_value_data";
CREATE TRIGGER "update_name_trigger"
AFTER
INSERT ON "key_value_data" FOR EACH ROW
    WHEN (NEW."path" = 'name') EXECUTE PROCEDURE "update_name"();
--
--
-- function and trigger to create or update vessel mmsi in static_data
CREATE OR REPLACE FUNCTION "update_mmsi"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "static_data" ("context", "mmsi")
VALUES (NEW."context", NEW."value_text") ON CONFLICT("context") DO
UPDATE
SET "mmsi" = NEW."value_text";
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';
DROP TRIGGER IF EXISTS "update_mmsi_trigger" ON "key_value_data";
CREATE TRIGGER "update_mmsi_trigger"
AFTER
INSERT ON "key_value_data" FOR EACH ROW
    WHEN (NEW."path" = 'mmsi') EXECUTE PROCEDURE "update_mmsi"();