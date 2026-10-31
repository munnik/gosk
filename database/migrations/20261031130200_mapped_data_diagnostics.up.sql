-- A mapped value's diagnostics (see message/diagnostics.go): a flat jsonb
-- object describing the reading - the raw register behind it, a status
-- bit, why it was rejected - or NULL when it has none, which is every row
-- written before this and most rows after it.
--
-- Nullable and without a default, so this is a catalogue change and not a
-- rewrite of the hypertables, compressed chunks included.
ALTER TABLE "mapped_data_matching_context" ADD COLUMN IF NOT EXISTS "diagnostics" JSONB;
ALTER TABLE "mapped_data_other_context" ADD COLUMN IF NOT EXISTS "diagnostics" JSONB;

-- The view's "*" was expanded when it was created, so it does not pick the
-- column up on its own. CREATE OR REPLACE may append a column to a view
-- without disturbing anything that depends on the ones it already has.
CREATE OR REPLACE VIEW "mapped_data" AS
    SELECT * FROM "mapped_data_matching_context"
    UNION ALL
    SELECT * FROM "mapped_data_other_context";
