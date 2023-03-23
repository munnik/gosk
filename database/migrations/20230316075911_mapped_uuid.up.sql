ALTER TABLE "mapped_data"
ADD COLUMN "mapped_uuid" uuid;

UPDATE "mapped_data" 
SET "mapped_uuid" = public.uuid_generate_v3(public.uuid_ns_dns(), to_char("time", 'YYYY-MM-DD HH24:MI:SS.US') || "context" || "origin" || "path");

ALTER TABLE "mapped_data"
ALTER COLUMN "mapped_uuid" SET NOT NULL;

CREATE INDEX IF NOT EXISTS "origin_mapped_uuid_time_idx" 
ON "mapped_data"("origin", "mapped_uuid", "time") WITH (timescaledb.transaction_per_chunk);

ALTER TABLE "mapped_data" 
ADD CONSTRAINT "origin_mapped_uuid_time_pkey" PRIMARY KEY USING INDEX "origin_mapped_uuid_time_idx";

ALTER TABLE "mapped_data"
ALTER COLUMN "mapped_uuid" SET NOT NULL;
