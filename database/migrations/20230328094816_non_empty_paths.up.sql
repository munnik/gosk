UPDATE "mapped_data" SET "value" = "value", "path" = 'name' WHERE "path" = '' AND "value" ? 'name';
UPDATE "mapped_data" SET "value" = "value", "path" = 'mmsi' WHERE "path" = '' AND "value" ? 'mmsi';

DROP TRIGGER "update_name_trigger" ON "mapped_data";
CREATE TRIGGER "update_name_trigger"
AFTER
INSERT ON "mapped_data" FOR EACH ROW
    WHEN (NEW."path" = 'name') EXECUTE PROCEDURE "update_name"();

DROP TRIGGER "update_mmsi_trigger" ON "mapped_data";
CREATE TRIGGER "update_mmsi_trigger"
AFTER
INSERT ON "mapped_data" FOR EACH ROW
    WHEN (NEW."path" = 'mmsi') EXECUTE PROCEDURE "update_mmsi"();

