UPDATE "mapped_data" SET "value" = "value", "path" = '' WHERE "path" = 'name';
UPDATE "mapped_data" SET "value" = "value", "path" = '' WHERE "path" = 'mmsi';

DROP TRIGGER "update_name_trigger" ON "mapped_data";
CREATE TRIGGER "update_name_trigger"
AFTER
INSERT ON "mapped_data" FOR EACH ROW
    WHEN (NEW."path" = '' AND NEW."value" ? 'name') EXECUTE PROCEDURE "update_name"();

DROP TRIGGER "update_mmsi_trigger" ON "mapped_data";
CREATE TRIGGER "update_mmsi_trigger"
AFTER
INSERT ON "mapped_data" FOR EACH ROW
    WHEN (NEW."path" = '' AND NEW."value" ? 'mmsi') EXECUTE PROCEDURE "update_mmsi"();
