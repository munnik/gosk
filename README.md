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

For communication between the different micro services [NNG](https://nng.nanomsg.org/) is used. The messages are serialized using JSON encoding.

#### 1.2.1 Raw JSON messages

The JSON message containts 3 fields:

1. time, number of nanoseconds elapsed since 00:00:00 UTC on 1 January 1970 (Epoch Time);
2. collector, identifier for the source of the data. Together with protocol_info this should contain all the required information for a mapper to map the data;
3. extra_info, extra information for the mapper to help map the data. This field is optional;
4. value, the actual value in base64 encoded format.

A message for a NMEA0183 $GPGLL string:
```json
{
  "time": 1579839222196901000,
  "collector": "Wheelhouse/GPS",
  "value": "JEdQR0xMLDM3MjMuMjQ3NSxOLDEyMTU4LjM0MTYsVywxNjEyMjkuNDg3LEEsQSo0MQ=="
}
```

A message for five modbus registers:
```json
{
  "time": 1579839222196901000,
  "collector": "EngineRoom/MainEngineStarboard",
  "extra_info": {
    "registers": [
      {
        "fc": 4,
        "address": 51300,
        "length": 1
      },
      {
        "fc": 4,
        "address": 51460,
        "length": 1
      },
      {
        "fc": 4,
        "address": 51426,
        "length": 1
      },
      {
        "fc": 4,
        "address": 51360,
        "length": 1
      },
      {
        "fc": 4,
        "address": 51606,
        "length": 1
      },
    ]
  },
  "value": "OTAzMzY3NDU5NzEyMTExNzQ0MzU="
}
```

#### 1.2.2 Mapped JSON messages

See https://signalk.org/specification/1.5.0/doc/data_model.html#delta-format

Example:
```json
{
  "context": "vessels.urn:mrn:imo:mmsi:234567890",
  "updates": [
    {
      "source": {
        "label": "N2000-01",
        "type": "NMEA2000",
        "src": "017",
        "pgn": 127488
      },
      "timestamp": "2010-01-07T07:18:44Z",
      "values": [
        {
          "path": "propulsion.0.revolutions",
          "value": 16.341667
        },
        {
          "path": "propulsion.0.boostPressure",
          "value": 45500
        }
      ]
    }
  ]
}
```

### 1.3. Storage

PostgreSQL and TimescaleDB are used as a time series database. Two tables are created. One table stores the raw data as received from the different collectors. The other table stores the mapped data as received from the different mappers.

#### 1.3.1. Raw data

```sql
CREATE TABLE "raw_data" (
    "time" TIMESTAMPTZ NOT NULL,
    "collector" TEXT NOT NULL,
    "extra_info" TEXT NOT NULL,
    "value" TEXT NOT NULL -- base64 encoded binary data
);
```

#### 1.3.2. Mapped data

```sql
CREATE TABLE "key_value_data" (
    "time" TIMESTAMPTZ NOT NULL,
    "context" TEXT NULL,
    "source" JSON NOT NULL,
    "path" TEXT NOT NULL,
    "value" JSON NOT NULL
);
```