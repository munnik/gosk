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
INSERT ON "key_value_data" FOR EACH ROW
    WHEN (NEW."_path" = 'name') EXECUTE PROCEDURE "update_name"();
--
--
-- function and trigger to create or update vessel mmsi in static_data
CREATE OR REPLACE FUNCTION "update_mmsi"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "static_data" ("_context", "_mmsi")
VALUES (NEW."_context", NEW."_value") ON CONFLICT("_context") DO
UPDATE
SET "_mmsi" = NEW."_value";
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';
DROP TRIGGER IF EXISTS "update_mmsi_trigger" ON "key_value_data";
CREATE TRIGGER "update_mmsi_trigger"
AFTER
INSERT ON "key_value_data" FOR EACH ROW
    WHEN (NEW."_path" = 'mmsi') EXECUTE PROCEDURE "update_mmsi"();