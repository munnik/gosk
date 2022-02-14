ALTER TABLE "key_value_data"
DROP COLUMN "uuid";

ALTER TABLE "raw_data"
DROP COLUMN "uuid";

DROP EXTENSION "uuid-ossp" CASCADE;
