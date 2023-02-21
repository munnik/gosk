CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA "public" CASCADE; 

ALTER TABLE "raw_data"
ADD COLUMN "uuid" UUID NOT NULL DEFAULT "public"."uuid_nil"();

ALTER TABLE "key_value_data"
ADD COLUMN "uuid" UUID NOT NULL DEFAULT "public"."uuid_nil"();
