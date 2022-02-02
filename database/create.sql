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
create extension if not exists timescaledb cascade;
drop table if exists raw_data;
create table raw_data (
    _time timestamptz not null,
    _key varchar [] not null,
    _value bytea not null
);
select create_hypertable('raw_data', '_time');
drop table if exists key_value_data;
create table key_value_data (
    _time timestamptz not null,
    _key varchar [] not null,
    _context varchar null,
    _path varchar not null,
    _value varchar not null
);
select create_hypertable('key_value_data', '_time');
drop table if exists static_data;
create table static_data (
    _context text not null primary key,
    _name text,
    _mmsi text
);
-- function and trigger to create or update vessel name in static_data
CREATE OR REPLACE FUNCTION "update_name"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "static_data" ("_context", "_name")
VALUES (NEW."_context", NEW."_value") ON CONFLICT("_context") DO
UPDATE
SET "_name" = NEW."_value";
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';
DROP TRIGGER IF EXISTS "update_name_trigger" ON "key_value_data";
CREATE TRIGGER "update_name_trigger"
AFTER
INSERT ON "key_value_data" FOR EACH ROW << << << < HEAD
    WHEN (NEW."path" = 'name') EXECUTE PROCEDURE "update_name"();
== == == =
WHEN (NEW."_path" = 'name') EXECUTE PROCEDURE "update_name"();
>> >> >> > serial_devices --
--
-- function and trigger to create or update vessel mmsi in static_data
CREATE OR REPLACE FUNCTION "update_mmsi"() RETURNS TRIGGER AS $$ BEGIN << << << < HEAD
INSERT INTO "static_data" ("context", "mmsi")
VALUES (NEW."context", NEW."value_text") ON CONFLICT("context") DO
UPDATE
SET "mmsi" = NEW."value_text";
== == == =
INSERT INTO "static_data" ("_context", "_mmsi")
VALUES (NEW."_context", NEW."_value") ON CONFLICT("_context") DO
UPDATE
SET "_mmsi" = NEW."_value";
>> >> >> > serial_devices RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';
DROP TRIGGER IF EXISTS "update_mmsi_trigger" ON "key_value_data";
CREATE TRIGGER "update_mmsi_trigger"
AFTER
INSERT ON "key_value_data" FOR EACH ROW << << << < HEAD
    WHEN (NEW."path" = 'mmsi') EXECUTE PROCEDURE "update_mmsi"();
== == == =
WHEN (NEW."_path" = 'mmsi') EXECUTE PROCEDURE "update_mmsi"();
>> >> >> > serial_devices