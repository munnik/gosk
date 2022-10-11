# GOSK

Go SignalK implementation and more.

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

The JSON message contains the following fields:

1. collector, identifier for the source of the data;
1. timestamp, the time when the data was receive in UTC and RFC3339Nano format;
1. uuid, used to link raw and mapped data together;
1. value, the actual value in base64 encoded format. This value contains all information needed for the mapper to map the data.

##### An example message for a NMEA0183 string:

```json
{
  "collector": "GPS",
  "timestamp": "2022-02-09T16:12:29.481162381Z",
  "uuid": "49175a7e-c4cb-455c-ae99-05eca8f7997d",
  "value": "JEdQR0xMLDM3MjMuMjQ3NSxOLDEyMTU4LjM0MTYsVywxNjEyMjkuNDg3LEEsQSo0MQ=="
}
```

The `value` is a base 64 encoded string of the NMEA018 sentence `$GPGLL,3723.2475,N,12158.3416,W,161229.487,A,A*41`.

##### An example message for two modbus registers:

```json
{
  "collector": "CAT 3512",
  "timestamp": "2022-02-09T12:03:57.431272983Z",
  "uuid": "c67ae38f-64c6-427d-a91d-282a25623cc2",
  "value": "AQAEyOwAAgABjnA="
}
```

The `value` is a base64 encoded string of the following bytes

- `0x01` The slave id
- `0x00 0x04` The function code
- `0xc8 0xec` The address of the first register
- `0x00 0x02` The number of registers
- `0x00 0x01` The value of the first register
- `0x8e 0x70` The value of the second register

#### 1.2.2 Mapped JSON messages

The basis is the [Signal K Delta format](https://signalk.org/specification/1.5.0/doc/data_model.html#delta-format) with some extras. The `label`, `type` and `src` properties in the `source` section are used as follows. `label` is filled with `name` property of the collector config, `type` is filled with protocol that is used and `src` is filled with the `url` property of the collector config. Each individual `value` section has an extra property `uuid` that is filled with the `uuid` of the corresponding raw data.

##### Example:

```json
{
  "context": "vessels.urn:mrn:imo:mmsi:234567890",
  "updates": [
    {
      "source": {
        "label": "CAT 3512",
        "type": "MODBUS",
        "src": "tcp://127.0.0.1:5020"
      },
      "timestamp": "2010-01-07T07:18:44Z",
      "values": [
        {
          "path": "propulsion.0.revolutions",
          "value": 16.341667,
          "uuid": "05a2ce62-6c52-4ce9-bb4f-07bb47cfa348"
        },
        {
          "path": "propulsion.0.boostPressure",
          "value": 45500,
          "uuid": "05a2ce62-6c52-4ce9-bb4f-07bb47cfa348"
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
