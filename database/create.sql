create extension if not exists timescaledb cascade;

drop table if exists raw_data;

create table raw_data (
    _time timestamptz not null,
    _key varchar[] not null,
    _value bytea not null
);

select create_hypertable('raw_data', '_time');

drop table if exists key_value_data;

create table key_value_data (
    _time timestamptz not null,
    _key varchar[] not null,
    _context varchar null,
    _path varchar not null,
    _value varchar not null
);

select create_hypertable('key_value_data', '_time');