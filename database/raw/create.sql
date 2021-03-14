drop table if exists raw_data;

create table raw_data (
    _time timestamptz not null,
    _key varchar[] not null,
    _value bytea not null
);

select create_hypertable('raw_data', '_time');