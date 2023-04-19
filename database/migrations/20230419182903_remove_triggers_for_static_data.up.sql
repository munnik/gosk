-- update static data will be handled be pg_cron

DROP TRIGGER IF EXISTS "update_beam_trigger" ON "mapped_data";
DROP TRIGGER IF EXISTS "update_callsignvhf_trigger" ON "mapped_data";
DROP TRIGGER IF EXISTS "update_eninumber_trigger" ON "mapped_data";
DROP TRIGGER IF EXISTS "update_length_trigger" ON "mapped_data";
DROP TRIGGER IF EXISTS "update_mmsi_trigger" ON "mapped_data";
DROP TRIGGER IF EXISTS "update_name_trigger" ON "mapped_data";
DROP TRIGGER IF EXISTS "update_vesseltype_trigger" ON "mapped_data";

DROP FUNCTION IF EXISTS "update_beam";
DROP FUNCTION IF EXISTS "update_callsignvhf";
DROP FUNCTION IF EXISTS "update_eninumber";
DROP FUNCTION IF EXISTS "update_length";
DROP FUNCTION IF EXISTS "update_mmsi";
DROP FUNCTION IF EXISTS "update_name";
DROP FUNCTION IF EXISTS "update_vesseltype";
