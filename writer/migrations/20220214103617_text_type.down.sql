DROP TRIGGER "update_name_trigger" ON "key_value_data";
DROP TRIGGER "update_mmsi_trigger" ON "key_value_data";
DROP TRIGGER "update_callsignvhf_trigger" ON "key_value_data";
ALTER TABLE "key_value_data"
ALTER COLUMN "path"
SET DATA TYPE CHARACTER VARYING USING "path"::CHARACTER VARYING;
ALTER TABLE "key_value_data"
ALTER COLUMN "context"
SET DATA TYPE CHARACTER VARYING USING "context"::CHARACTER VARYING;
ALTER TABLE "key_value_data"
ALTER COLUMN "key"
SET DATA TYPE CHARACTER VARYING [] USING string_to_array("key", '.');
ALTER TABLE "raw_data"
ALTER COLUMN "key"
SET DATA TYPE CHARACTER VARYING [] USING string_to_array("key", '.');
CREATE TRIGGER "update_name_trigger"
AFTER
INSERT ON "key_value_data" FOR EACH ROW
    WHEN (NEW."path" = 'name') EXECUTE PROCEDURE "update_name"();
CREATE TRIGGER "update_mmsi_trigger"
AFTER
INSERT ON "key_value_data" FOR EACH ROW
    WHEN (NEW."path" = 'mmsi') EXECUTE PROCEDURE "update_mmsi"();
CREATE TRIGGER "update_callsignvhf_trigger"
AFTER
INSERT ON "key_value_data" FOR EACH ROW
    WHEN (NEW."path" = 'communication.callsignVhf') EXECUTE PROCEDURE "update_callsignvhf"();
