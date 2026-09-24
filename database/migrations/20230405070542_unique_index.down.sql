-- The up migration creates "mapped_data_unique_idx". This used to drop
-- "mapped_data_context_path_connector_time_idx", a name nothing ever
-- creates, so the downgrade failed here and left the schema dirty.
DROP INDEX IF EXISTS "mapped_data_unique_idx";
