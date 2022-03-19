UPDATE "mapped_data" SET "value" = "value"->'name', "path" = 'name' WHERE "path" = '' AND "value" ? 'name';
UPDATE "mapped_data" SET "value" = "value"->'mmsi', "path" = 'mmsi' WHERE "path" = '' AND "value" ? 'mmsi';

CREATE OR REPLACE FUNCTION "update_name"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "static_data" ("context", "name")
VALUES (NEW."context", NEW."value") ON CONFLICT("context") DO
UPDATE
SET "name" = NEW."value";
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';
DROP TRIGGER "update_name_trigger" ON "mapped_data";
CREATE TRIGGER "update_name_trigger"
AFTER
INSERT ON "mapped_data" FOR EACH ROW
    WHEN (NEW."path" = 'name') EXECUTE PROCEDURE "update_name"();
CREATE OR REPLACE FUNCTION "update_mmsi"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "static_data" ("context", "mmsi")
VALUES (NEW."context", NEW."value") ON CONFLICT("context") DO
UPDATE
SET "mmsi" = NEW."value";
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';
DROP TRIGGER "update_mmsi_trigger" ON "mapped_data";
CREATE TRIGGER "update_mmsi_trigger"
AFTER
INSERT ON "mapped_data" FOR EACH ROW
    WHEN (NEW."path" = 'mmsi') EXECUTE PROCEDURE "update_mmsi"();
CREATE OR REPLACE FUNCTION "update_callsignvhf"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "static_data" ("context", "callsignvhf")
VALUES (NEW."context", NEW."value") ON CONFLICT("context") DO
UPDATE
SET "callsignvhf" = NEW."value";
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';
DROP TRIGGER "update_callsignvhf_trigger" ON "mapped_data";
CREATE TRIGGER "update_callsignvhf_trigger"
AFTER
INSERT ON "mapped_data" FOR EACH ROW
    WHEN (NEW."path" = 'communication.callsignVhf') EXECUTE PROCEDURE "update_callsignvhf"();
UPDATE "static_data"
SET "callsignvhf" = "value"
FROM "mapped_data"
WHERE "static_data"."context" = "mapped_data"."context"
    AND "mapped_data"."path" = 'communication.callsignVhf';