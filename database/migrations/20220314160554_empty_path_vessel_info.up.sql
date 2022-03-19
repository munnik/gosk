UPDATE "mapped_data" SET "value" = replace('{"name": #}', '#', "value"::TEXT)::JSONB, "path" = '' WHERE "path" = 'name';
UPDATE "mapped_data" SET "value" = replace('{"mmsi": #}', '#', "value"::TEXT)::JSONB, "path" = '' WHERE "path" = 'mmsi';
UPDATE "static_data" SET "name" = trim(both '"' FROM "name"), "mmsi" = trim(both '"' FROM "mmsi"), "callsignvhf" = trim(both '"' FROM "callsignvhf");

CREATE OR REPLACE FUNCTION "update_name"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "static_data" ("context", "name")
VALUES (NEW."context", NEW."value"->>'name') ON CONFLICT("context") DO
UPDATE
SET "name" = NEW."value"->>'name';
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';
DROP TRIGGER "update_name_trigger" ON "mapped_data";
CREATE TRIGGER "update_name_trigger"
AFTER
INSERT ON "mapped_data" FOR EACH ROW
    WHEN (NEW."path" = '' AND NEW."value" ? 'name') EXECUTE PROCEDURE "update_name"();

CREATE OR REPLACE FUNCTION "update_mmsi"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "static_data" ("context", "mmsi")
VALUES (NEW."context", NEW."value"->>'mmsi') ON CONFLICT("context") DO
UPDATE
SET "mmsi" = NEW."value"->>'mmsi';
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';
DROP TRIGGER "update_mmsi_trigger" ON "mapped_data";
CREATE TRIGGER "update_mmsi_trigger"
AFTER
INSERT ON "mapped_data" FOR EACH ROW
    WHEN (NEW."path" = '' AND NEW."value" ? 'mmsi') EXECUTE PROCEDURE "update_mmsi"();

CREATE OR REPLACE FUNCTION "update_callsignvhf"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "static_data" ("context", "callsignvhf")
VALUES (NEW."context", trim(both '"' FROM NEW."value"::TEXT)) ON CONFLICT("context") DO
UPDATE
SET "callsignvhf" = trim(both '"' FROM NEW."value"::TEXT);
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';
DROP TRIGGER "update_callsignvhf_trigger" ON "mapped_data";
CREATE TRIGGER "update_callsignvhf_trigger"
AFTER
INSERT ON "mapped_data" FOR EACH ROW
    WHEN (NEW."path" = 'communication.callsignVhf') EXECUTE PROCEDURE "update_callsignvhf"();
