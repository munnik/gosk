DROP TRIGGER "update_eninumber_trigger" ON "mapped_data";
DROP FUNCTION "update_eninumber"();
ALTER TABLE "static_data" DROP COLUMN "eninumber";

DROP TRIGGER "update_length_trigger" ON "mapped_data";
DROP FUNCTION "update_length"();
ALTER TABLE "static_data" DROP COLUMN "length";

DROP TRIGGER "update_beam_trigger" ON "mapped_data";
DROP FUNCTION "update_beam"();
ALTER TABLE "static_data" DROP COLUMN "beam";

DROP TRIGGER "update_vesseltype_trigger" ON "mapped_data";
DROP FUNCTION "update_vesseltype"();
ALTER TABLE "static_data" DROP COLUMN "vesseltype";
