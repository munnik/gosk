CREATE TABLE "static_data" (
    "_context" TEXT NOT NULL PRIMARY KEY,
    "_name" TEXT,
    "_mmsi" TEXT
);
CREATE OR REPLACE FUNCTION "update_name"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "static_data" ("_context", "_name")
VALUES (NEW."_context", NEW."_value") ON CONFLICT("_context") DO
UPDATE
SET "_name" = NEW."_value";
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';
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
CREATE TRIGGER "update_mmsi_trigger"
AFTER
INSERT ON "key_value_data" FOR EACH ROW
    WHEN (NEW."_path" = 'mmsi') EXECUTE PROCEDURE "update_mmsi"();