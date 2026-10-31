# Diagnostics: what a value says about its sensor

Design notes for the `diagnostics` field on a mapped value (see
`message/diagnostics.go` for the type and `MappingConfig.Diagnostics` in
`config/main.go` for how a mapping produces one). They record a rule, a
design for sensors whose state arrives separately from their readings, and
the open points, so that the implementation can be checked against them.

## The rule

A Signal K path is a statement about the vessel: the torque on a shaft, the
power of an engine, its speed, a temperature. The `diagnostics` object next
to a value is a statement about the sensor that produced that value: how far
to trust it, and why.

Sensor state never becomes a Signal K path. A rotor's supply voltage, a
telemetry link's glitch rate, a modbus module's connected-inputs bitmask are
not properties of the vessel, and once they are paths nobody can tell asset
data from sensor data. They travel in the column of the value they qualify,
and their history is the history of those rows.

Identity - serials, software versions, configured settings - is neither. It
belongs to the tool that validates a sensor's configuration, except for the
stable identifier that says which hardware produced a reading (below).

## The object

Two levels: the common keys (below) at the top, the same for every kind of
sensor, and `details` - the one key whose value is an object - holding what
only this kind of sensor reports. Every other entry, at either level, is a
leaf: a finite number, a bool or a string (`NormalizeDiagnostic`); `details`
holds no object of its own, and anything deeper is dropped. Numbers are in SI
units, as in Signal K: seconds, volts, amperes, kelvin, ratios rather than
percentages. A name never carries a unit (`imageAge`, not `imageAgeSeconds`;
`supplyCurrent`, not `supplyCurrentmA`), so a producer reporting in other
units is converted by the mapping. Nor does a name repeat the object it is in
(`status`, not `diagnosticsStatus`). Names have stable types; a change of
meaning gets a new name. A mapping expression that evaluates to nil omits that
key - omission means "not known at the time", never "fine". Absence must never
be read as healthy, and frame-derived facts (which sensors measured) are never
dropped because some slower diagnostics are unavailable.

## Common keys

Most keys belong to one kind of sensor and help only someone who knows it:
a rotor's raw voltage, a loop current, a glitch rate. A few answer the same
question for every sensor, so a consumer can ask it across a fleet without
knowing what is behind a path:

| Key | Type | Question |
|---|---|---|
| `sensor` | string | Which hardware produced this value? |
| `connected` | bool | Does the sensor answer at all? |
| `trust` | number, 0 to 1 | How far can this value be relied on? |
| `rejected` | string | Why is there no value? (reserved, set by the mapper) |
| `status`, `age` | string, seconds | How old is the evidence behind the above? |
| `details` | object of leaves | What else does this kind of sensor report? |

`sensor` and `connected` are required on every mapping that configures
diagnostics. `status` and `age` exist only where the sensor's state arrives
on a separate diagnostics input (below); a sensor whose value and state
arrive in the same message has no older evidence to date.

Everything else goes in `details`: a rotor's raw voltage and image age, a
loop current, a glitch rate, which rotors measured. A query across sensor
types reads the top level; a query about one sensor reads
`diagnostics->'details'->>'rotor1RawVoltage'`. A mapping configures the two
levels side by side, `diagnostics:` for the common keys and
`diagnosticDetails:` for the details, each a map of names to expressions;
`details` and `rejected` cannot be configured as top-level names.

```json
{
  "sensor": "port-shaft-meter",
  "connected": true,
  "trust": 0.7,
  "status": "fresh",
  "age": 0.4,
  "details": {
    "torqueSelection": "sensor1",
    "sensor1Valid": true,
    "sensor2Valid": false,
    "rotor1RawVoltage": 4.8,
    "rotor1ImageAge": 0.5,
    "supplyCurrent": 0.299,
    "shutdownCounter": 263
  }
}
```

### Trust

`trust` is a ratio: 0 means the value cannot be relied on at all, 1 means it
is certainly correct, and anything between says there is some uncertainty.
It has two parts, both the mapping's to configure:

- **What the source is.** Some sources are better than others at the same
  quantity: a calibrated flow meter measures fuel flow, an engine controller
  calculates it. The mapping for each states its baseline, so that two
  sources of one quantity compare - the meter's flow carries a higher
  `trust` than the engine's.
- **What the reading shows.** The mapping lowers the baseline from the
  incoming data: a shaft power meter measuring with one rotor instead of
  two (the value no longer cancels the shaft's bending), a loop current
  near the edge of its range, stale diagnostics.

A trust is set by an expression, like any other key, and is clamped to 0..1;
a mapping that cannot judge leaves it out rather than guess 1. A value with
`trust` 0 is still a value - the sensor answered and the mapper kept the
reading - whereas `rejected` means there is no value at all. The number
compares between sources of the same quantity and between readings of one
sensor over time; it is not a calibrated probability, and the same number
on two unrelated paths means only that both mappings judged alike.

## Two kinds of sensor state, one object

Some sensor state is in the same message as the value. A shaft power meter's
frame carries both rotor words, the combined word, speed and temperature; a
modbus reply carries the raw register. The mapper that produces the value
evaluates the mapping's `diagnostics` and `diagnosticDetails` expressions over
that same input; the details here are `sensor1Valid`, `sensor2Valid`,
`validSensors`, `errorValue`, the raw words, and `torqueSelection`
(`bothMean`, `sensor1`, `sensor2`: how the value was chosen). Every mapper
that produces values evaluates them; today only the modbus mapper does, the
binary and json mappers still need the same evaluator wired in.

Other sensor state arrives on its own, slower: the meter's proxy decodes the
meter's memory images and emits one JSON line a second with per-rotor supply
voltage and threshold, address-glitch rate, image age, rotor temperature,
stator supply current and shutdown counter. A modbus module's connected-inputs
bitmask, polled slowly, is the same kind of thing. These are the **sensor
diagnostics** of the fast readings, arriving as a stream of diagnostics
reports.

## Sensor diagnostics: joined in the mapper

The join point is the map stage. The connector stays an acquisition process
and the slow line gets its own line-based connector and raw topic; the
aggregate keeps its one rule (a reading replaces a reading); the writer and
the query side never have to reconstruct an association after the fact. The
mapper is the one place that knows which input produced which value and can
attach the evidence atomically.

- `map` already accepts repeated subscribe URLs and feeds them to one
  subscriber. A diagnostics input is declared in the mapper's configuration
  with the connector identity it comes from, its timestamp field and a
  freshness limit, and routed by that identity - never by sniffing the
  payload, and never keyed by the raw message UUID, which is fresh per
  message.
- The mapper keeps the latest parsed diagnostics report per source, with
  the producer's report time and the local monotonic receipt time, and a
  small bounded history so the two streams can interleave. The join is "the
  latest report no later than the reading", to the accuracy the timestamps
  allow; an older report never replaces a newer state; no reading waits for
  diagnostics and no emitted reading is rewritten when a report arrives
  late.
- The latest report is exposed to the mapping's `diagnostics` and
  `diagnosticDetails` expressions as the variable `diagnostics`, with its age
  (not `context`, which already means the Signal K vessel context in
  `MapperConfig`). Consuming a diagnostics report that produces no value is
  normal, not an empty-mapping warning.
- Startup and restart are simple: a report before the first reading is
  cached; readings before any report flow with their diagnostics marked
  missing; a restart starts empty; diagnostics missing for minutes stay
  visibly missing and never block the value. The proxy's periodic full
  reports repopulate the cache on the next line. A retained message, where
  a transport retains one, keeps its original timestamp and is not fresh
  because it was just delivered.

### Freshness is explicit, at two levels

Every reading with a diagnostics input carries `status` (`fresh`, `stale`,
`missing`) and, once any report has been seen, `age` in seconds - both
about the latest diagnostics report, not about the value. For never-seen
diagnostics the age is omitted, since neither zero nor a sentinel is true.
A stale report omits the leaves it would have supplied (the voltages, the
currents) but keeps the status, so a row can say "the diagnostics were
stale" rather than leave it to be inferred from missing keys.

A fresh report can carry stale evidence: the proxy keeps the last memory
image across a reconnect to the meter, with its age. So a rotor's effective
image age is the report's image age plus the time from the report to the
reading, judged against its own limit, per rotor and for the stator.

## Rate limiting: sampled trends, forwarded transitions

The rate limiter is time based and keeps one row per path per interval. The
diagnostics travel with the sampled reading, so the slow trends (a voltage
sliding towards its threshold over weeks) are in the rows and nothing more
is needed for them.

A change of sensor state between two samples would be invisible, so a small,
configured set of transitions passes the limiter at once, as extra rows
beside the periodic ones: per-rotor validity (tracked per rotor, since a
count of one cannot tell rotor 1 from rotor 2), rejection and recovery,
`connected`, diagnostics freshness status, sensor identity, and `trust`
crossing a configured step. Never the whole object - ages,
rates and voltages change on every row and would switch the limiter off.
Degradation passes immediately, recovery is debounced, flapping is bounded.
A transition the mapper never saw (a dip between two one-second reports) is
not captured; that needs counters or extrema on the producer's side.

When no sensor measured - both torque words zero - the mapper emits a
rejected reading (no value, `rejected` in the diagnostics) rather than
nothing, so that downstream expressions stop using the last good value. A
silent stream still needs liveness monitoring of its own; diagnostics reports
never manufacture readings.

## Identity

A configured, stable identifier in `sensor`, and per-component identities
where the sensor has replaceable parts (`rotor1Sensor`, `rotor2Sensor`, as
strings, in `details`), so that two sensors feeding one path over time can be
told apart and a replaced rotor is visible. Rotor 1 and 2 are channel
positions; the component id is the hardware. An identity read from an old
cached image is not adopted as the identity of newly connected hardware.
Longer term this belongs in source or provenance metadata; the column is the
bridge.

## Derived values

A derived value (power from torque and speed) carries a compact, configured
summary of its inputs' evidence, evaluated by the aggregate mapper over the
same environment as the value expression: the torque's sensor identity,
validity and diagnostics status, the speed's source and age, with
operand-prefixed keys, and its own `trust`. A derived value is no more
trustworthy than its least trusted input, so its trust defaults to the
minimum of its inputs' and a mapping may lower it further. Not every
rotor voltage copied onto every derived path - that duplicates trends and
pretends the derived value has one sensor. Freshness of a derived value
includes the age of its input readings. A diagnostics-only report does not
produce a new derived sample; the next real input reading carries the
updated evidence. An in-place transform of a value keeps its diagnostics.

## Signal K

Provenance is Signal K's `$source`; identity will move there once gosk has
a durable representation. `meta` describes a path's presentation and its
`timeout` the value's expiry; dynamic sensor state stays out of it. The
writers leave `diagnostics` out of Signal K deltas, so a Signal K consumer
does not learn that a non-null value is degraded. Notifications are paths,
so the rule as stated excludes them for sensor state; gosk already publishes
connector-connected notifications, and whether that remains an allowed
exception is a decision to take explicitly, not one this design should
quietly make.

## Storage and queries

The jsonb column on the sampled rows is the first design. A trend query
constrains origin, vessel context, path and time, then extracts and casts
one key; it returns freshness and identity alongside and plots unavailable
values as gaps. Longer views bucket to minutes or hours with minima, maxima
and coverage. Series-and-time access is what such a query needs; a GIN
index over the objects is not its first optimisation.

Do not assume repeated objects compress well in a columnstore: ages and
rates make every object distinct, and JSON leaves do not become independent
columns. Measure representative objects, their compressed size and the
week-and-month queries before widening the vocabulary.

A side table keyed by sensor identity and observation time becomes
necessary when sensor health must be recorded while no readings arrive at
all; a side table that merely splits the per-reading object away from its
row adds joins and consistency work for nothing.

## Cost before storage

The limiter samples; everything before it carries every reading. At two
thousand frames a second, one kilobyte of diagnostics per reading is
megabytes a second per sensor through mapping, aggregation and transport.
Parse each diagnostics report once, cache what depends only on it, build
the frame-dependent leaves per frame, treat an emitted object as immutable,
keep the vocabulary to what is used, and profile this path before adding to
it.

## Order of work

1. Diagnostics input in the map stage with explicit freshness.
2. `diagnostics` evaluation in the binary and json mappers; nil omits.
3. Transition forwarding through the rate limiter.
4. Rejected readings when nothing measured.
5. Stable identity leaves.
6. Configured diagnostics on derived values in the aggregate.

Deferred until a requirement asks for them: a persistent diagnostics cache,
historical backfilling, generic diagnostics-only messages in the aggregate, a
provenance graph, a sensor hypertable.

Risks the design is built around: attaching the wrong sensor's diagnostics,
presenting old images as fresh, losing failure transitions, and the load
added before the limiter.
