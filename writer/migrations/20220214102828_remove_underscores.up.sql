ALTER TABLE "raw_data"
    RENAME COLUMN "_time" TO "time";
ALTER TABLE "raw_data"
    RENAME COLUMN "_key" TO "key";
ALTER TABLE "raw_data"
    RENAME COLUMN "_value" TO "value";
ALTER TABLE "key_value_data"
    RENAME COLUMN "_time" TO "time";
ALTER TABLE "key_value_data"
    RENAME COLUMN "_key" TO "key";
ALTER TABLE "key_value_data"
    RENAME COLUMN "_context" TO "context";
ALTER TABLE "key_value_data"
    RENAME COLUMN "_path" TO "path";
ALTER TABLE "key_value_data"
    RENAME COLUMN "_value" TO "value";
ALTER TABLE "static_data"
    RENAME COLUMN "_context" TO "context";
ALTER TABLE "static_data"
    RENAME COLUMN "_name" TO "name";
ALTER TABLE "static_data"
    RENAME COLUMN "_mmsi" TO "mmsi";
CREATE OR REPLACE FUNCTION "update_name"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "static_data" ("context", "name")
VALUES (NEW."context", NEW."value") ON CONFLICT("context") DO
UPDATE
SET "name" = NEW."value";
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';
DROP TRIGGER "update_name_trigger" ON "key_value_data";
CREATE TRIGGER "update_name_trigger"
AFTER
INSERT ON "key_value_data" FOR EACH ROW
    WHEN (NEW."path" = 'name') EXECUTE PROCEDURE "update_name"();
CREATE OR REPLACE FUNCTION "update_mmsi"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "static_data" ("context", "mmsi")
VALUES (NEW."context", NEW."value") ON CONFLICT("context") DO
UPDATE
SET "mmsi" = NEW."value";
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';
DROP TRIGGER "update_mmsi_trigger" ON "key_value_data";
CREATE TRIGGER "update_mmsi_trigger"
AFTER
INSERT ON "key_value_data" FOR EACH ROW
    WHEN (NEW."path" = 'mmsi') EXECUTE PROCEDURE "update_mmsi"();
