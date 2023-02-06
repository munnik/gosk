ALTER TABLE "remote_data" 
DROP COLUMN "local";

ALTER TABLE "remote_data" 
DROP COLUMN "end";

ALTER TABLE "remote_data"
RENAME TO "transfer_remote_data";
