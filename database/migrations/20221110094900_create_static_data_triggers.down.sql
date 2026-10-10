DROP TRIGGER update_beam_trigger ON mapped_data;
DROP TRIGGER update_callsignvhf_trigger ON mapped_data;
DROP TRIGGER update_eninumber_trigger ON mapped_data;
DROP TRIGGER update_length_trigger ON mapped_data;
DROP TRIGGER update_mmsi_trigger ON mapped_data;
DROP TRIGGER update_name_trigger ON mapped_data;
DROP TRIGGER update_vesseltype_trigger ON mapped_data;

-- These used to be DROP FUNCTION, but the up migration only CREATE OR
-- REPLACEd them: all seven already existed, created by
-- 20220214102722_static_data, 20220214103511_callsignvhf,
-- 20220314160554_empty_path_vessel_info and
-- 20220320130840_extra_static_data_properties, and
-- 20220906084811_drop_static_data_triggers dropped only the triggers.
-- Dropping them here over-reversed the migration and broke the next one
-- down the chain: 20220906084811's down migration recreates those same
-- seven triggers and failed with 'function update_name() does not exist'.
--
-- What the up migration actually changed was the schema qualification -
-- it rewrote the bodies to write "gosk"."static_data" instead of
-- "static_data". So restore the unqualified bodies as
-- 20220314160554/20220320130840 left them, and leave the functions in
-- place for the migrations below to drop.
CREATE OR REPLACE FUNCTION "update_name"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "static_data" ("context", "name")
VALUES (NEW."context", NEW."value"->>'name') ON CONFLICT("context") DO
UPDATE
SET "name" = NEW."value"->>'name';
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';

CREATE OR REPLACE FUNCTION "update_mmsi"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "static_data" ("context", "mmsi")
VALUES (NEW."context", NEW."value"->>'mmsi') ON CONFLICT("context") DO
UPDATE
SET "mmsi" = NEW."value"->>'mmsi';
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';

CREATE OR REPLACE FUNCTION "update_callsignvhf"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "static_data" ("context", "callsignvhf")
VALUES (NEW."context", trim(both '"' FROM NEW."value"::TEXT)) ON CONFLICT("context") DO
UPDATE
SET "callsignvhf" = trim(both '"' FROM NEW."value"::TEXT);
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';

CREATE OR REPLACE FUNCTION "update_eninumber"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "static_data" ("context", "eninumber")
VALUES (NEW."context", trim(both '"' FROM NEW."value"::TEXT)) ON CONFLICT("context") DO
UPDATE
SET "eninumber" = trim(both '"' FROM NEW."value"::TEXT);
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';

CREATE OR REPLACE FUNCTION "update_length"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "static_data" ("context", "length")
VALUES (NEW."context", (NEW."value"->'overall')::DOUBLE PRECISION) ON CONFLICT("context") DO
UPDATE
SET "length" = (NEW."value"->'overall')::DOUBLE PRECISION;
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';

CREATE OR REPLACE FUNCTION "update_beam"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "static_data" ("context", "beam")
VALUES (NEW."context", NEW."value"::DOUBLE PRECISION) ON CONFLICT("context") DO
UPDATE
SET "beam" = NEW."value"::DOUBLE PRECISION;
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';

CREATE OR REPLACE FUNCTION "update_vesseltype"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "static_data" ("context", "vesseltype")
VALUES (NEW."context", NEW."value"->>'name') ON CONFLICT("context") DO
UPDATE
SET "vesseltype" = NEW."value"->>'name';
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';
