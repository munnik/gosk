CREATE OR REPLACE VIEW "transfer_data" AS
SELECT 
    "transfer_remote_data"."origin", 
    "transfer_remote_data"."start",
    "transfer_local_data"."local" AS "local_count",
    "transfer_remote_data"."count" AS "remote_count"
FROM 
    "transfer_remote_data" 
INNER JOIN 
    "transfer_local_data" 
    ON "transfer_local_data"."start" = "transfer_remote_data"."start" 
    AND "transfer_local_data"."origin" = "transfer_remote_data"."origin"
WHERE 
    "transfer_remote_data"."start" BETWEEN (SELECT MIN("start") FROM "transfer_local_data") AND (SELECT MAX("start") FROM "transfer_local_data")
UNION
SELECT 
    "transfer_remote_data"."origin", 
    "transfer_remote_data"."start",
    0 AS "local_count",
    "transfer_remote_data"."count" AS "remote_count"
FROM 
    "transfer_remote_data" 
WHERE 
    "transfer_remote_data"."start" BETWEEN (SELECT MIN("start") FROM "transfer_local_data") AND (SELECT MAX("start") FROM "transfer_local_data")
    AND NOT EXISTS (
        SELECT 
            1 
        FROM 
            "transfer_local_data" 
        WHERE 
            "transfer_local_data"."start" = "transfer_remote_data"."start" 
            AND "transfer_local_data"."origin" = "transfer_remote_data"."origin"
    )
;