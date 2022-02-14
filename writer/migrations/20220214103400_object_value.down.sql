INSERT INTO "key_value_data"
SELECT *
FROM (
        SELECT "time",
            "key",
            "context",
            'navigation.position.altitude' AS "path",
            "value"->'altitude' AS "value"
        FROM "key_value_data"
        WHERE "path" = 'navigation.position'
        UNION
        SELECT "time",
            "key",
            "context",
            'navigation.position.latitude' AS "path",
            "value"->'latitude' AS "value"
        FROM "key_value_data"
        WHERE "path" = 'navigation.position'
        UNION
        SELECT "time",
            "key",
            "context",
            'navigation.position.longitude' AS "path",
            "value"->'longitude' AS "value"
        FROM "key_value_data"
        WHERE "path" = 'navigation.position'
    ) "subquery"
WHERE "value" IS NOT NULL;
DELETE FROM "key_value_data"
WHERE "path" = 'navigation.position';
INSERT INTO "key_value_data"
SELECT *
FROM (
        SELECT "time",
            "key",
            "context",
            'design.length.overall' AS "path",
            "value"->'overall' AS "value"
        FROM "key_value_data"
        WHERE "path" = 'design.length'
        UNION
        SELECT "time",
            "key",
            "context",
            'design.length.hull' AS "path",
            "value"->'hull' AS "value"
        FROM "key_value_data"
        WHERE "path" = 'design.length'
        UNION
        SELECT "time",
            "key",
            "context",
            'design.length.waterline' AS "path",
            "value"->'waterline' AS "value"
        FROM "key_value_data"
        WHERE "path" = 'design.length'
    ) "subquery"
WHERE "value" IS NOT NULL;
DELETE FROM "key_value_data"
WHERE "path" = 'design.length';
