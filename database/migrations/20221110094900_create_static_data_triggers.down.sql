DROP TRIGGER update_beam_trigger ON mapped_data;
DROP TRIGGER update_callsignvhf_trigger ON mapped_data;
DROP TRIGGER update_eninumber_trigger ON mapped_data;
DROP TRIGGER update_length_trigger ON mapped_data;
DROP TRIGGER update_mmsi_trigger ON mapped_data;
DROP TRIGGER update_name_trigger ON mapped_data;
DROP TRIGGER update_vesseltype_trigger ON mapped_data;

DROP FUNCTION "update_beam"();
DROP FUNCTION "update_callsignvhf"();
DROP FUNCTION "update_eninumber"();
DROP FUNCTION "update_length"();
DROP FUNCTION "update_mmsi"();
DROP FUNCTION "update_name"();
DROP FUNCTION "update_vesseltype"();
