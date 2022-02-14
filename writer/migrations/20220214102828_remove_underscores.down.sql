ALTER TABLE "raw_data"
    RENAME COLUMN "time" TO "_time";
ALTER TABLE "raw_data"
    RENAME COLUMN "key" TO "_key";
ALTER TABLE "raw_data"
    RENAME COLUMN "value" TO "_value";
ALTER TABLE "key_value_data"
    RENAME COLUMN "time" TO "_time";
ALTER TABLE "key_value_data"
    RENAME COLUMN "key" TO "_key";
ALTER TABLE "key_value_data"
    RENAME COLUMN "context" TO "_context";
ALTER TABLE "key_value_data"
    RENAME COLUMN "path" TO "_path";
ALTER TABLE "key_value_data"
    RENAME COLUMN "value" TO "_value";
ALTER TABLE "static_data"
    RENAME COLUMN "context" TO "_context";
ALTER TABLE "static_data"
    RENAME COLUMN "name" TO "_name";
ALTER TABLE "static_data"
    RENAME COLUMN "mmsi" TO "_mmsi";
CREATE OR REPLACE FUNCTION "update_name"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "static_data" ("_context", "_name")
VALUES (NEW."_context", NEW."_value") ON CONFLICT("_context") DO
UPDATE
SET "_name" = NEW."_value";
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';
DROP TRIGGER "update_name_trigger" ON "key_value_data";
CREATE TRIGGER "update_name_trigger"
AFTER
INSERT ON "key_value_data" FOR EACH ROW
    WHEN (NEW."_path" = 'name') EXECUTE PROCEDURE "update_name"();
CREATE OR REPLACE FUNCTION "update_mmsi"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "static_data" ("_context", "_mmsi")
VALUES (NEW."_context", NEW."_value") ON CONFLICT("_context") DO
UPDATE
SET "_mmsi" = NEW."_value";
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';
DROP TRIGGER "update_mmsi_trigger" ON "key_value_data";
CREATE TRIGGER "update_mmsi_trigger"
AFTER
INSERT ON "key_value_data" FOR EACH ROW
    WHEN (NEW."_path" = 'mmsi') EXECUTE PROCEDURE "update_mmsi"();
