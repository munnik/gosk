ALTER TABLE "key_value_data"
ALTER COLUMN "value"
SET DATA TYPE JSONB USING CASE
        WHEN "path" IN (
            'mmsi',
            'name',
            'communication.callsignVhf',
            'navigation.state',
            'registrations.other.eni.registration'
        ) THEN (
            '"' || TRIM(
                BOTH ''''
                FROM quote_literal("value")
            ) || '"'
        )::JSONB
        ELSE "value"::JSONB
    END;
