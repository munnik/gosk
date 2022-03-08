CREATE TABLE "key_value_data" (
    "_time" TIMESTAMP WITH TIME ZONE NOT NULL,
    "_key" CHARACTER VARYING [] NOT NULL,
    "_context" CHARACTER VARYING NULL,
    "_path" CHARACTER VARYING NOT NULL,
    "_value" CHARACTER VARYING NOT NULL
);
SELECT create_hypertable('key_value_data', '_time');
