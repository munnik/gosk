-- Puts the columns back, empty: every row gets the nil uuid, which is what
-- rows without a source message always carried. The values that were dropped
-- are not recoverable and nothing reads them.
-- The view's grants are kept across that: people read gosk.mapped_data
-- directly (read access per person, granted by hand), and a recreated view
-- starts with none. They are saved before the drop and granted again after,
-- in this migration's transaction, so they cannot be lost half way. They
-- are carried in a transaction-local setting rather than a table: CREATE
-- TABLE AS on a database with the columnstore on crashes the server
-- (timescale/timescaledb#10655, see 20261031130300).
SELECT set_config('gosk.mapped_data_acl',
    COALESCE((SELECT "relacl"::text FROM "pg_catalog"."pg_class" WHERE "oid" = to_regclass('mapped_data')), ''),
    true);

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

DO $$
DECLARE
    grant_ record;
BEGIN
    FOR grant_ IN
        SELECT acl.privilege_type, acl.grantee
        FROM aclexplode(NULLIF(current_setting('gosk.mapped_data_acl'), '')::aclitem[]) acl
        WHERE acl.grantee <> acl.grantor
    LOOP
        EXECUTE format('GRANT %s ON "mapped_data" TO %s', grant_.privilege_type,
            CASE WHEN grant_.grantee = 0 THEN 'PUBLIC' ELSE quote_ident(pg_get_userbyid(grant_.grantee)) END);
    END LOOP;
END $$;
