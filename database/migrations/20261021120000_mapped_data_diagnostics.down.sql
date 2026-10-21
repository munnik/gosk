-- CREATE OR REPLACE cannot take a column off a view, so the view is
-- recreated as the split migration left it, then the column is dropped.
DROP VIEW IF EXISTS "mapped_data";
ALTER TABLE "mapped_data_matching_context" DROP COLUMN IF EXISTS "diagnostics";
ALTER TABLE "mapped_data_other_context" DROP COLUMN IF EXISTS "diagnostics";
CREATE VIEW "mapped_data" AS
    SELECT * FROM "mapped_data_matching_context"
    UNION ALL
    SELECT * FROM "mapped_data_other_context";
