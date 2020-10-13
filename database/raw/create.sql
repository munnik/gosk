drop table raw_data;

create table raw_data (
    _time timestamptz not null,
    _key varchar[] not null,
    _value bytea not null
);

alter table raw_data owner to gosk;

select create_hypertable('raw_data', '_time');