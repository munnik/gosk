CREATE OR REPLACE VIEW "transfer_data" AS
SELECT 
    "transfer_remote_data"."origin", 
    "transfer_remote_data"."start", 
    COALESCE("transfer_local_data"."count", 0) AS "local_count", 
    "transfer_remote_data"."count" AS "remote_count"
FROM "transfer_local_data" 
RIGHT JOIN "transfer_remote_data" ON "transfer_local_data"."start" = "transfer_remote_data"."start" AND "transfer_local_data"."origin" = "transfer_remote_data"."origin"
WHERE "transfer_remote_data"."start" BETWEEN (SELECT MIN("start") FROM "transfer_local_data") AND (SELECT MAX("start") FROM "transfer_local_data");
