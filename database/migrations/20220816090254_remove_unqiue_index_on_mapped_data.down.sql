CREATE UNIQUE INDEX "mapped_data_unique_rows_idx" ON "mapped_data" ("time", "collector", "type", "context", "path", "value", "uuid", "origin");
