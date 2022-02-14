ALTER TABLE "static_data"
ADD COLUMN "callsignvhf" TEXT;
CREATE OR REPLACE FUNCTION "update_callsignvhf"() RETURNS TRIGGER AS $$ BEGIN
INSERT INTO "static_data" ("context", "callsignvhf")
VALUES (NEW."context", NEW."value") ON CONFLICT("context") DO
UPDATE
SET "callsignvhf" = NEW."value";
RETURN NEW;
END;
$$ LANGUAGE 'plpgsql';
CREATE TRIGGER "update_callsignvhf_trigger"
AFTER
INSERT ON "key_value_data" FOR EACH ROW
    WHEN (NEW."path" = 'communication.callsignVhf') EXECUTE PROCEDURE "update_callsignvhf"();
UPDATE "static_data"
SET "callsignvhf" = "value"
FROM "key_value_data"
WHERE "static_data"."context" = "key_value_data"."context"
    AND "key_value_data"."path" = 'communication.callsignVhf';
