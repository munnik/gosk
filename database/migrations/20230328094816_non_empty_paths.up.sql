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

