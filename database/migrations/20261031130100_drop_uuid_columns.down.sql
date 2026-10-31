-- Puts the columns back, empty: every row gets the nil uuid, which is what
-- rows without a source message always carried. The values that were dropped
-- are not recoverable and nothing reads them.
DROP VIEW IF EXISTS "mapped_data";

ALTER TABLE "mapped_data_matching_context"
    ADD COLUMN "uuid" UUID NOT NULL DEFAULT "public"."uuid_nil"(),
    ADD COLUMN "transfer_uuid" UUID;

ALTER TABLE "mapped_data_other_context"
    ADD COLUMN "uuid" UUID NOT NULL DEFAULT "public"."uuid_nil"(),
    ADD COLUMN "transfer_uuid" UUID;

ALTER TABLE "raw_data"
    ADD COLUMN "uuid" UUID NOT NULL DEFAULT "public"."uuid_nil"();

CREATE INDEX IF NOT EXISTS "mapped_data_origin_uuid_time_idx" ON "mapped_data_matching_context" ("origin", "uuid", "time");

CREATE VIEW "mapped_data" AS
    SELECT * FROM "mapped_data_matching_context"
    UNION ALL
    SELECT * FROM "mapped_data_other_context";
