DROP TRIGGER "update_callsignvhf_trigger" ON "key_value_data";
DROP FUNCTION "update_callsignvhf";
ALTER TABLE "static_data" DROP COLUMN "callsignvhf";
