ALTER TABLE "remote_data" 
DROP COLUMN "local",
DROP COLUMN "end",
DROP COLUMN "count_requests",
DROP COLUMN "last_count_request",
DROP COLUMN "data_requests",
DROP COLUMN "last_data_request";

ALTER TABLE "remote_data" 
RENAME COLUMN "remote" TO "count";

ALTER TABLE "remote_data"
RENAME TO "transfer_remote_data";
