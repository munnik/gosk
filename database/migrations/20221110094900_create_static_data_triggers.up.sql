CREATE TRIGGER "update_name_trigger"
AFTER
INSERT ON "mapped_data" FOR EACH ROW
    WHEN (NEW."path" = '' AND NEW."value" ? 'name') EXECUTE PROCEDURE "update_name"();

CREATE TRIGGER "update_mmsi_trigger"
AFTER
INSERT ON "mapped_data" FOR EACH ROW
    WHEN (NEW."path" = '' AND NEW."value" ? 'mmsi') EXECUTE PROCEDURE "update_mmsi"();

CREATE TRIGGER "update_callsignvhf_trigger"
AFTER
INSERT ON "mapped_data" FOR EACH ROW
    WHEN (NEW."path" = 'communication.callsignVhf') EXECUTE PROCEDURE "update_callsignvhf"();

CREATE TRIGGER "update_eninumber_trigger"
AFTER
INSERT ON "mapped_data" FOR EACH ROW
    WHEN (NEW."path" = 'registrations.other.eni.registration') EXECUTE PROCEDURE "update_eninumber"();

CREATE TRIGGER "update_length_trigger"
AFTER
INSERT ON "mapped_data" FOR EACH ROW
    WHEN (NEW."path" = 'design.length' AND NEW."value" ? 'overall') EXECUTE PROCEDURE "update_length"();

CREATE TRIGGER "update_beam_trigger"
AFTER
INSERT ON "mapped_data" FOR EACH ROW
    WHEN (NEW."path" = 'design.beam') EXECUTE PROCEDURE "update_beam"();

CREATE TRIGGER "update_vesseltype_trigger"
AFTER
INSERT ON "mapped_data" FOR EACH ROW
    WHEN (NEW."path" = 'design.aisShipType' AND NEW."value" ? 'name') EXECUTE PROCEDURE "update_vesseltype"();
    