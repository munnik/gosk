CREATE UNIQUE INDEX IF NOT EXISTS "mapped_data_unique_idx" ON "mapped_data"("time", "origin", "context", "path", "connector");