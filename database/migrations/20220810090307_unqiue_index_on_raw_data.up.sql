CREATE UNIQUE INDEX "raw_data_unique_rows_idx" ON "raw_data" ("time", "collector", "value", "uuid", "type");
