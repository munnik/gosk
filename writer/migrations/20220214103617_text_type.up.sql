DROP TRIGGER "update_name_trigger" ON "key_value_data";
DROP TRIGGER "update_mmsi_trigger" ON "key_value_data";
DROP TRIGGER "update_callsignvhf_trigger" ON "key_value_data";
ALTER TABLE "raw_data"
ALTER COLUMN "key"
SET DATA TYPE TEXT USING array_to_string("key", '.');
ALTER TABLE "key_value_data"
ALTER COLUMN "key"
SET DATA TYPE TEXT USING array_to_string("key", '.');
ALTER TABLE "key_value_data"
ALTER COLUMN "context"
SET DATA TYPE TEXT USING "context"::TEXT;
ALTER TABLE "key_value_data"
ALTER COLUMN "path"
SET DATA TYPE TEXT USING "path"::TEXT;
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
