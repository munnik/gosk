ALTER TABLE "key_value_data"
ALTER COLUMN "value"
SET DATA TYPE CHARACTER VARYING USING CASE
        WHEN "path" IN (
            'mmsi',
            'name',
            'communication.callsignVhf',
            'navigation.state',
            'registrations.other.eni.registration'
        ) THEN TRIM(
            BOTH '"'
            FROM "value"::CHARACTER VARYING
        )
        ELSE "value"::CHARACTER VARYING
    END;
        