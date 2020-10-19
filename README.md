# GOSK <!-- omit in toc -->

Go SignalK implementation and more.
- [1. Design](#1-design)
  - [1.1. Micro services](#11-micro-services)
    - [1.1.1. Collectors](#111-collectors)
    - [1.1.2. Raw store](#112-raw-store)
    - [1.1.3. Mappers](#113-mappers)
    - [1.1.4. Mapped store](#114-mapped-store)
    - [1.1.5. Transporters](#115-transporters)
    - [1.1.6. Publish](#116-publish)
  - [1.2. Communication between micro services](#12-communication-between-micro-services)
    - [1.2.1. Header](#121-header)
    - [1.2.2. Timestamp](#122-timestamp)
    - [1.2.3. Body](#123-body)
  - [1.3. Storage](#13-storage)
    - [1.3.1. Raw data](#131-raw-data)
    - [1.3.2. Mapped data](#132-mapped-data)

## 1. Design

This implementation consist of several small programs that collect, map, store and publish sensor data from one or more vessels.

### 1.1. Micro services

GOSK is a set of micro services that collect, map, store, transport and publish data available on a ship. GOSK publishes data in the [SignalK Open Marine Data Standard](https://signalk.org/). All data is stored in both a raw format (before any mapping is done) and in a mapped format.

#### 1.1.1. Collectors

The only role of a collector is to collect raw data from the sensors on board. Collectors are made for specific protocols, e.g. a collector for NMEA0183 over UDP knows how to listen for data and a collector for Modbus over TCP knows how to pull data. Collectors have their individual configuration. Multiple collectors for the same protocol/transport can coexist in a system, e.g. 2 Canbus collectors for a port side and starboard engine.

#### 1.1.2. Raw store

The only role of a raw store is to store the raw data in a time series database. See below for the storage format.

#### 1.1.3. Mappers

The only role of a mapper is to translate data from a raw format to a mapped format. Mappers should not be aware of transport protocols, e.g. data coming from NMEA0183 over RS-422 and NMEA0183 over UDP should be handled by the same mapper.

#### 1.1.4. Mapped store

The only role of a mapped store is to store the mapped data in a time series database. See below for the storage format.

#### 1.1.5. Transporters

Transporters move (subsets) of mapped data from one network to another network. Transporters can be used to send data from the vessel to the cloud. Currently no transporters are implemented.

#### 1.1.6. Publish

A publisher can provide the mapped data to other applications in different data formats and transport protocols. Currently a publisher for [SignalK REST API](https://signalk.org/specification/1.4.0/doc/rest_api.html) and [SignalK Streaming API](https://signalk.org/specification/1.4.0/doc/streaming_api.html).

### 1.2. Communication between micro services

For communication between the different micro services [NNG](https://nng.nanomsg.org/) is used. Each message is a string of bytes and consists of a header, timestamp and the message body separated by a nul character. `<header>\x00<timestamp>\x00<message body>`. Only the first two nul characters are handled as separators so the message body can contain nul characters.

#### 1.2.1. Header

The header consist of one or more header segments `<header segment>/<header segment>/<header segment>`, each header segment is separated by a forward slash. Header segments are strings and cannot contain a nul character or a forward slash. The header should give enough information to parse the message body.

#### 1.2.2. Timestamp

The timestamp is the number of nanoseconds elapsed since January 1, 1970 UTC.

#### 1.2.3. Body

The message body contains the data, the header describes the type of message.

### 1.3. Storage

PostgreSQL 12 and TimescaleDB are used as a time series database. Two tables are created. One table stores the raw data as received from the different collectors. The other table stores the mapped data as received from the different mappers.

#### 1.3.1. Raw data

```sql
CREATE TABLE public.raw_data
(
    _time timestamp without time zone NOT NULL,
    _key character varying[] COLLATE pg_catalog."default" NOT NULL,
    _value bytea NOT NULL
)
```

#### 1.3.2. Mapped data

```sql
CREATE TABLE public.key_value_data
(
    _time timestamp without time zone NOT NULL,
    _key character varying[] COLLATE pg_catalog."default" NOT NULL,
    _context character varying COLLATE pg_catalog."default",
    _path character varying COLLATE pg_catalog."default" NOT NULL,
    _value character varying COLLATE pg_catalog."default" NOT NULL
)
```