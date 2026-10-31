-- CREATE OR REPLACE cannot take a column off a view, so the view is
-- recreated as the split migration left it, then the column is dropped.
-- Recreating the view loses its grants; they are saved and granted again,
-- as in 20261031130100.
SELECT set_config('gosk.mapped_data_acl',
    COALESCE((SELECT "relacl"::text FROM "pg_catalog"."pg_class" WHERE "oid" = to_regclass('mapped_data')), ''),
    true);

DROP VIEW IF EXISTS "mapped_data";
ALTER TABLE "mapped_data_matching_context" DROP COLUMN IF EXISTS "diagnostics";
ALTER TABLE "mapped_data_other_context" DROP COLUMN IF EXISTS "diagnostics";
CREATE VIEW "mapped_data" AS
    SELECT * FROM "mapped_data_matching_context"
    UNION ALL
    SELECT * FROM "mapped_data_other_context";

DO $$
DECLARE
    grant_ record;
BEGIN
    FOR grant_ IN
        SELECT acl.privilege_type, acl.grantee, acl.is_grantable
        FROM aclexplode(NULLIF(current_setting('gosk.mapped_data_acl'), '')::aclitem[]) acl
        WHERE acl.grantee <> acl.grantor
    LOOP
        EXECUTE format('GRANT %s ON "mapped_data" TO %s%s', grant_.privilege_type,
            CASE WHEN grant_.grantee = 0 THEN 'PUBLIC' ELSE quote_ident(pg_get_userbyid(grant_.grantee)) END,
            CASE WHEN grant_.is_grantable THEN ' WITH GRANT OPTION' ELSE '' END);
    END LOOP;
END $$;
