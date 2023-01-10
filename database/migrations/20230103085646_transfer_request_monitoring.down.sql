ALTER TABLE "remote_data" 
DROP COLUMN "count_requests",
DROP COLUMN "last_count_request",
DROP COLUMN "data_requests",
DROP COLUMN "last_data_request";

DROP TABLE "transfer_log";

ALTER TABLE "mapped_data" 
DROP COLUMN "transfer_uuid";