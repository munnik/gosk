INSERT INTO "key_value_data"
SELECT "time",
    "key",
    "context",
    "path",
    (
        trim(
            trailing ','
            FROM '{' || CASE
                    WHEN "altitude" IS NULL THEN ''
                    ELSE '"altitude":' || "altitude" || ','
                END || CASE
                    WHEN "latitude" IS NULL THEN ''
                    ELSE '"latitude":' || "latitude" || ','
                END || CASE
                    WHEN "longitude" IS NULL THEN ''
                    ELSE '"longitude":' || "longitude" || ','
                END || ''
        ) || '}'
    )::JSONB AS "value"
FROM (
        SELECT "time",
            "key",
            "context",
            'navigation.position' AS "path",
            MAX("value"::double precision) FILTER (
                WHERE "path" = 'navigation.position.altitude'
            ) AS "altitude",
            MAX("value"::double precision) FILTER (
                WHERE "path" = 'navigation.position.latitude'
            ) AS "latitude",
            MAX("value"::double precision) FILTER (
                WHERE "path" = 'navigation.position.longitude'
            ) AS "longitude"
        FROM "key_value_data"
        WHERE "path" IN (
                'navigation.position.altitude',
                'navigation.position.latitude',
                'navigation.position.longitude'
            )
        GROUP BY "time",
            "key",
            "context"
    ) "subquery";
DELETE FROM "key_value_data"
WHERE "path" IN (
        'navigation.position.altitude',
        'navigation.position.latitude',
        'navigation.position.longitude'
    );
INSERT INTO "key_value_data"
SELECT "time",
    "key",
    "context",
    "path",
    (
        trim(
            trailing ','
            FROM '{' || CASE
                    WHEN "overall" IS NULL THEN ''
                    ELSE '"overall":' || "overall" || ','
                END || CASE
                    WHEN "hull" IS NULL THEN ''
                    ELSE '"hull":' || "hull" || ','
                END || CASE
                    WHEN "waterline" IS NULL THEN ''
                    ELSE '"waterline":' || "waterline" || ','
                END || ''
        ) || '}'
    )::JSONB AS "value"
FROM (
        SELECT "time",
            "key",
            "context",
            'design.length' AS "path",
            MAX("value"::double precision) FILTER (
                WHERE "path" = 'design.length.overall'
            ) AS "overall",
            MAX("value"::double precision) FILTER (
                WHERE "path" = 'design.length.hull'
            ) AS "hull",
            MAX("value"::double precision) FILTER (
                WHERE "path" = 'design.length.waterline'
            ) AS "waterline"
        FROM "key_value_data"
        WHERE "path" IN (
                'design.length.overall',
                'design.length.hull',
                'design.length.waterline'
            )
        GROUP BY "time",
            "key",
            "context"
    ) "subquery";
DELETE FROM "key_value_data"
WHERE "path" IN (
        'design.length.overall',
        'design.length.hull',
        'design.length.waterline'
    );
