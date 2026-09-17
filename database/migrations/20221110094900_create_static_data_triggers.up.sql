CREATE OR REPLACE FUNCTION "update_name"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "gosk"."static_data" ("context", "name")
VALUES (NEW."context", NEW."value"->>'name') ON CONFLICT("context") DO
UPDATE
SET "name" = NEW."value"->>'name';
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';

CREATE OR REPLACE FUNCTION "update_mmsi"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "gosk"."static_data" ("context", "mmsi")
VALUES (NEW."context", NEW."value"->>'mmsi') ON CONFLICT("context") DO
UPDATE
SET "mmsi" = NEW."value"->>'mmsi';
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';

CREATE OR REPLACE FUNCTION "update_callsignvhf"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "gosk"."static_data" ("context", "callsignvhf")
VALUES (NEW."context", trim(both '"' FROM NEW."value"::TEXT)) ON CONFLICT("context") DO
UPDATE
SET "callsignvhf" = trim(both '"' FROM NEW."value"::TEXT);
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';

CREATE OR REPLACE FUNCTION "update_eninumber"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "gosk"."static_data" ("context", "eninumber")
VALUES (NEW."context", trim(both '"' FROM NEW."value"::TEXT)) ON CONFLICT("context") DO
UPDATE
SET "eninumber" = trim(both '"' FROM NEW."value"::TEXT);
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';

CREATE OR REPLACE FUNCTION "update_length"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "gosk"."static_data" ("context", "length")
VALUES (NEW."context", (NEW."value"->'overall')::DOUBLE PRECISION) ON CONFLICT("context") DO
UPDATE
SET "length" = (NEW."value"->'overall')::DOUBLE PRECISION;
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';

CREATE OR REPLACE FUNCTION "update_beam"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "gosk"."static_data" ("context", "beam")
VALUES (NEW."context", NEW."value"::DOUBLE PRECISION) ON CONFLICT("context") DO
UPDATE
SET "beam" = NEW."value"::DOUBLE PRECISION;
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';

CREATE OR REPLACE FUNCTION "update_vesseltype"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "gosk"."static_data" ("context", "vesseltype")
VALUES (NEW."context", NEW."value"->>'name') ON CONFLICT("context") DO
UPDATE
SET "vesseltype" = NEW."value"->>'name';
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';

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
