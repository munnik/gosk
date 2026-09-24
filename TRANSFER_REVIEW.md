# Review: vessel → cloud data transfer

A review of the data transfer / reconciliation system (`transfer/`, the transfer
queries in `database/postgresql.go`, and the MQTT read/write path), with
recommendations.

**Short version:** the transfer system is a reconciliation loop bolted on top of
a transport that is configured to lose data. Fix the transport first — most of
the gaps the reconciliation loop chases are self-inflicted.

**Status:** §1, §2 and §6 are implemented (see "Deploying §1" below for the two
prerequisites this cannot enforce from the code). §3 and §4 are unchanged
proposals; nothing in them has been built. §7 is a decision, recorded so it is
not revisited from scratch; it changed no code, and §7.9 revisits it against
the microsecond-accurate UUIDs §6 ended up with.

---

## 1. The primary data path is fire-and-forget — *implemented*

`writer/mqtt.go:59-61` publishes every batch of deltas with **QoS 0,
retained=true**. And `mqtt.createClientOptions` (`mqtt/main.go:93-107`) sets no
`ClientID` and no `SetCleanSession(false)`.

Consequences:

- **QoS 0** — no PUBACK, no retransmit. A TCP connection that is degrading
  (the normal state on a vessel link, not the exceptional one) drops messages
  silently. paho hands them to a socket that is about to die, and they are gone.
- **No ClientID + clean session** — the broker cannot hold a persistent session.
  When the cloud-side reader (`reader/mqtt.go`) is down or disconnected, nothing
  is queued for it. When the vessel reconnects, nothing in flight is resumed.
- **retained=true on `vessels/<mmsi>`** — only the *last* batch is kept, and it
  is replayed to every new subscriber. That is not durability, it is a
  duplicate-injection mechanism.

### What was done

1. Stable `ClientID` plus `SetCleanSession(false)`. `mqtt.New` takes a `role`
   argument and derives `gosk-<role>-<hostname>`; `client_id` in the
   configuration overrides it. The id is logged on every connection.
2. **QoS 1** on the data path (`writer/mqtt.go`, `dataQoS`) and on the transfer
   protocol (`transfer/main.go`, `transferQoS`).
3. A disk-backed store, enabled by setting `store_dir`, one subdirectory per
   client id.
4. `retained` dropped on the data topic and on both transfer topics.
5. The store is bounded by `store_max_size` (default 256 MiB) with oldest-first
   eviction of outbound messages — `mqtt/store.go`, `boundedFileStore`.

Two further changes the above turned out to need:

- **`Publish` no longer waits on the token.** At QoS 0 a token completes once
  the packet reaches the network; at QoS 1 it completes on the broker's PUBACK,
  so waiting would have blocked the writer's flush for the entire length of an
  outage — the opposite of the point, which is that paho holds the message and
  redelivers it while the caller moves on. Delivery is reported asynchronously
  by one bounded watcher goroutine.
- **`SetMaxResumePubInFlight(100)`.** paho's default is unlimited, so a vessel
  reconnecting after a day would hand its entire backlog to the link that just
  came back, in one burst.

This turns "connection flapped for 20 minutes" from a permanent gap into a
delayed delivery. The reconciliation system becomes a rare backstop for the
cases MQTT genuinely cannot cover (broker-side loss, DB write failure, store
overflow), which is what it should be.

The same QoS 0 had applied to the transfer protocol itself
(`transfer/request.go`, `transfer/respond.go`), with retransmitted data going
back out through the same QoS 0 writer — **the mechanism that repairs lost data
was itself sent over a lossy channel, and a repair could fail exactly as
silently as the original.** That is fixed along with the rest.

### Deploying §1

Two prerequisites that live outside this repository:

- **The broker has to be willing to hold the sessions.** mosquitto's
  `max_queued_messages` defaults to 1000, far too small for a vessel's worth of
  deltas, and `persistence` has to be on for a queue to survive a broker
  restart. Without both, the broker silently drops what it was asked to keep.
- **A duplicate client id is now a hard failure mode.** Two clients sharing one
  take turns disconnecting each other, which looks like a flaky link rather than
  a misconfiguration. The derived id is unique as long as a given role runs once
  per host; set `client_id` explicitly where that does not hold.

---

## 2. Concrete bugs and liabilities in the current transfer code — *fixed*

Line numbers below refer to the code as it was when this was written
(`29e2b851`), not to the fixed version.

### 2.1 `sendDataRequests` leaks goroutines and amplifies requests

`transfer/request.go:195-216`. `wg.Add(1)` covers only the sleeper goroutine; the
per-origin goroutines never call `Done` and are not counted. So the function
returns after `sleepBetweenDataRequests` regardless of whether they have finished
feeding `dataRequestChannel` (capacity = `numberOfRequestWorkers`, i.e. 2). The
outer `for { t.sendDataRequests() }` then spawns a *second* full set for the same
origins, which blocks on the same channel, and so on.

With `max_periods_to_request: 100000` in `config/transfer/request.yaml` and 2
workers each doing a query + publish + log insert, the producers can never keep
up — goroutines and duplicate data requests accumulate indefinitely.

Compare `sendCountRequests` (`transfer/request.go:86-88`), which does the
`wg.Add(len(origins)+1)` correctly.

**Fixed:** every per-origin goroutine is now counted and `defer wg.Done()`s.

### 2.2 `injectData` sends twice the UUIDs it means to

`transfer/respond.go:147-150`. `make([]uuid.UUID, len(...))` followed by `append`
produces N nil UUIDs plus N real ones. Should be
`make([]uuid.UUID, 0, len(...))`.

**Fixed.**

### 2.3 The count-request loop is O(all periods since the vessel's first data), every cycle

`transfer/request.go:137` walks from `MIN(start)` in 5-minute steps up to one
hour ago. With the 180-day retention set by
`20250115144655_continuous_aggregate_policy_longer_period`, that is ~52,000
iterations per origin.

Every period lacking a `transfer_remote_data` row produces one MQTT publish *and*
one synchronous `LogTransferRequest` round trip (`database/postgresql.go:608`). A
vessel that has been offline for a week generates ~2,000 of those every 30
minutes, forever — and each one overwrites the same retained message on
`request/<origin>`, so the vessel gets one arbitrary stale request replayed on
reconnect rather than the backlog.

**Fixed**, in two parts. The number of count requests per origin per cycle is
capped by `max_count_requests_per_cycle` (default 1000), asking about the newest
periods first, so a genuine backlog drains a cycle at a time while a gap that
opened an hour ago is still closed at the first opportunity. And
`LogTransferRequest` no longer costs a round trip per request: it queues onto
the batch `Write*`/`updateStaticData` already use. It takes an explicit
timestamp now, because the row is written at flush time and `NOW()` would record
that rather than the event. The retained flag is gone with the rest of §1.

### 2.4 `transfer_log` has no retention policy

Every request and response is a row; nothing ever drops it. Add
`add_retention_policy('transfer_log', ...)`.

**Fixed:** migration `20260924120000_transfer_log_retention`, 90 days.

### 2.5 `mapped_data_other_context` is missing the index the transfer queries need

This was written up as "`SelectCountPerUuid` probably cannot use the index,
worth an `EXPLAIN`". It is more specific than that, and does not need an
`EXPLAIN` to establish.

`20230203163908_origin_uuid_index` created `mapped_data_origin_uuid_time_idx` on
what was then the single `mapped_data` table. `20231024113638_split_mapped_data`
renamed that table to `mapped_data_matching_context` — which carried its indexes
along — and created `mapped_data_other_context` with
`CREATE TABLE ... AS TABLE ... WITH NO DATA`, which copies neither indexes nor
constraints. Only the unique index was then recreated explicitly.

So the index has been missing from half the mapped data since October 2023. Both
`SelectCountPerUuid` and the responder's `ReadMapped` go through the
`mapped_data` view, a `UNION ALL` over both tables, so every transfer query has
been paying for a scan of the `other_context` half. That is a plausible
explanation for the 5-minute statement timeout in `request.yaml`.

**Fixed:** migration
`20260924120100_mapped_data_other_context_origin_uuid_index`.

### 2.6 Minor

- `i > t.maxPeriodsToRequest` should be `>=` (`transfer/request.go:206`).
- `firstPeriodRequested` is set twice on the break path.
- `golang.org/x/exp/rand` is deprecated — use `math/rand/v2`. Also
  `rand.Intn(int(t.sleepBetweenCountRequests))` panics if that duration is 0.

**Fixed**, all three; the random sleep goes through a `randomDuration` helper
that returns 0 for a non-positive interval.

---

## 3. The algorithmic problem: counts are the wrong primitive — *not addressed*

§2.3 bounds how much work the count-based scheme does per cycle. It does not
make counts a better primitive, which is the point below.

Comparing row counts per 5-minute bucket tells you *that* something is missing,
never *what*. Therefore:

- The data request must carry the entire `uuid → count` map for the bucket
  (potentially thousands of entries, as uncompressed JSON, at QoS 0) just to
  narrow things down.
- The responder resends every row for any UUID whose count differs. One missing
  row out of fifty costs fifty rows of satellite bandwidth.
- `completeness_factor: 0.98` means you deliberately tolerate 2% permanent loss
  to stop the loop spinning — which tells you the primitive does not converge.
- State grows as O(origins × buckets) forever in `transfer_remote_data`, and the
  `transfer_data` view (`20231024113638_split_mapped_data`) re-derives
  `MIN`/`MAX` over the continuous aggregates on every execution.

The right primitive is a **monotonic sequence with an acknowledged watermark**:
O(1) state per vessel, exact rather than approximate, and gap detection is a
subtraction instead of a 52,000-iteration loop.

---

## 4. Does PostgreSQL do this better? — *open question, nothing built*

### 4.1 Logical replication

**Yes — logical replication is exactly this mechanism, built in.** A replication
slot on the vessel is a durable, ordered, resumable cursor. The cloud subscriber
reconnects and resumes at the exact LSN it left off. No counting, no buckets, no
reconciliation protocol, no `completeness_factor`. The VPN (the `10.24.0.x`
addressing in the configs) means the cloud can reach the vessel, which is the
direction logical replication needs.

The caveats are real, and one is fundamental:

- **WAL retention is the whole game.** While a subscriber is disconnected, the
  slot pins WAL on the vessel. A three-week outage on a vessel with a small disk
  fills `pg_wal` and takes Postgres down — a worse failure than losing data. You
  cap it with `max_slot_wal_keep_size`, but then the slot is *invalidated* when
  exceeded, and recovery requires a full resync. **So a reconciliation backstop
  for long outages is still needed.** Logical replication shrinks the problem
  from "the normal case" to "the rare case"; it does not eliminate it. Still a
  large win, but budget for it rather than deleting `transfer/` outright.
- **Hypertables are the fiddly part.** Chunks are separate tables; publishing a
  hypertable directly means keeping new chunks in the publication as they are
  created, and the subscriber side has to match. Verify the exact mechanics
  against your TimescaleDB version before committing to a design — this is where
  these projects usually stall. The clean way around it: **replicate a plain
  (non-hyper) staging/outbox table** and let a cloud-side worker insert into the
  hypertable. That also bounds WAL retention to the outbox's own churn.
- **N:1 into one table** works (multiple subscriptions writing to the same table
  is supported on PG15+), and the PK `(time, origin, context, connector, path)`
  makes cross-vessel conflicts essentially impossible. Use `disable_on_error` so
  a conflict surfaces instead of wedging silently.
- **DDL is not replicated** — the `golang-migrate` migrations need to run on both
  ends in a coordinated order.

**What not to reach for:** TimescaleDB multi-node / distributed hypertables. It
was deprecated in 2.13 and removed shortly after; it is not a path forward.

### 4.2 The middle option — recommended first

A **transactional outbox on the vessel**, shipped over the existing MQTT link:

1. The vessel writes each mapped row to `outbox(seq bigserial, payload)` in the
   same transaction as the hypertable insert.
2. The shipper reads `WHERE seq > last_acked`, publishes in batches (zstd, as
   `writer/mqtt.go` already does for the main path), at QoS 1.
3. The cloud, on insert, records the highest contiguous `seq` per origin and
   publishes an ack.
4. The vessel deletes acked rows and advances its watermark.

This gives exact delivery semantics, O(1) reconciliation state, natural batching,
and no Postgres replication machinery, TimescaleDB version coupling, WAL pinning,
or cloud→vessel connectivity requirement. It is also a strictly smaller change
than logical replication: the wire format and the MQTT plumbing already exist.

---

## 5. Suggested order of work

1. ~~**MQTT durability**~~ — done, see §1. Highest value per line changed.
2. ~~**The bugs in §2**~~ — done.
3. **Measure what is left.** This is the next step, and it gates everything
   after it. `gosk_transfer_data_requests_total` and
   `gosk_transfer_count_requests_total` should fall sharply once §1 is deployed
   and the broker is configured to hold sessions; `gosk_transfer_missing_counts_total`
   (new) shows the per-origin backlog of periods without a remote count. If the
   residual gap rate is low enough, steps 4 and 5 are not worth doing.
4. **Outbox + watermark**, replacing count-based reconciliation (§3, §4.2).
5. **Logical replication** only if step 4 proves insufficient (§4.1) — and with
   an explicit plan for slot invalidation.

### What §1 and §2 did not change

- Counts remain the reconciliation primitive, with everything §3 says about
  them. `completeness_factor` still encodes a tolerated permanent loss rate.
- `transfer_remote_data` still grows as O(origins × buckets) with no retention.
- The per-request `uuid → count` map is still sent uncompressed, though it is
  now sent reliably.
- A data request still resends every row for any UUID whose count differs.

---

## 6. UUIDv7 for the identifiers gosk generates — *implemented*

Every raw message carries a generated UUID (`message.NewRaw`). It is written to
`raw_data.uuid`, follows the mapping into `mapped_data.uuid`, and is indexed
there by `mapped_data_origin_uuid_time_idx (origin, uuid, time)` — the index
§2.5 turned out to be missing from half the data. Those UUIDs were version 4:
122 bits of randomness, so consecutive messages produce values that sort
nowhere near each other.

Every generation site now calls `uuid.Must(uuid.NewV7())`, which returns version 7:
a 48-bit millisecond timestamp in the high bits, then a 12-bit sequence
counter, then randomness. Values generated in sequence sort in the order they
were created. `uuid.Must` panics if the system's source of randomness fails —
the same behaviour, from the same cause, as `uuid.New`'s for version 4.

### The library

gosk uses `github.com/munnik/uuid/v5`, a fork of `gofrs/uuid` carrying one
addition: UUIDv7 generators that fill `rand_a` with sub-millisecond time
(RFC 9562 method 3) rather than with a counter. `NewV7Precise` for the
clock, `NewV7AtTimePrecise` for a time that came from elsewhere.

That fork exists because no maintained library does this. `google/uuid`
derives `rand_a` from the sub-millisecond nanoseconds but on a scale
nothing else reads (256ns units, where the format's readers expect
4096ths of a millisecond). `gofrs/uuid` states outright that it implements
method 1 — `rand_a` is a counter with no time in it. The libraries that do
implement method 3 are all single-author packages, two of them already
archived. gofrs declined method 3 upstream in issue #174 on API-fit
grounds, so the fork is long-term rather than a staging post.

Why it matters here: the shaft power meter samples at 2kHz, one every
500µs, and a millisecond-resolution timestamp cannot tell two of those
apart. §7.7 has what the database can read back.

The call is written out at each site rather than hidden behind a helper, so
which version is in use is visible where it matters.

The rule is enforced mechanically rather than by convention: `.golangci.yml`
configures `forbidigo` to reject `uuid.New`, `uuid.NewString`, `uuid.NewRandom`
(all version 4) and `uuid.NewUUID` (version 1), each with a message pointing
back here. `git-hooks.hooks.golangci-lint` in `devenv.nix` runs it pre-commit
over the directories whose Go files changed.

Every other linter is off (`default: none`). That is deliberate — gosk has
never been linted, so enabling golangci-lint's defaults would surface a
backlog and block every commit until it was worked through. Turning more on is
a separate decision from this one.

Worth noting that the implementation is **strictly monotonic**, not just
timestamp-prefixed: `getV7Time` keeps a counter in the 12 bits below the
timestamp, so two UUIDs created in the same millisecond still sort in creation
order. That is what the index-locality argument below actually rests on. The
ordering is per process; two processes writing to one table interleave at
millisecond granularity, which is as fine as the index needs.

### What this buys, and what it does not

**Index locality — immediate, and the real reason to do it.** With random
UUIDs, each insert targets a different part of the UUID index, so the pages
being dirtied are spread across the whole index rather than clustered: more
page splits, and a write working set far larger than the rows actually being
inserted. Ordered UUIDs append near one edge.

**Compression — real but deferred.** A batch of ordered UUIDs shares its
leading bytes where random ones share nothing, so the column compresses better.
This only pays off on chunks that are actually compressed, and **no migration
in this repository sets up a compression or columnstore policy**. Until that
changes the compression benefit is zero. It is worth measuring against the
columnstore work rather than assuming a figure.

**Not retroactive.** Rows already written keep their version 4 UUIDs, so an
index stays as fragmented as it already is until those chunks age out.
`raw_data` turns over inside its 7-day retention. `mapped_data` has no
retention policy at all, so it will hold a mix indefinitely.

**Not smaller.** A UUID is still 16 bytes.

**Resolution.** Since the move to method 3 the embedded time resolves to
about 244ns, finer than the microsecond `timestamptz` itself stores, so the
UUID is no longer the limiting factor. Verified end to end: a row written
through gosk's own database layer came back with its `time` column and the
timestamp extracted from its UUID agreeing to the microsecond. §7.8 has the
caveat that applied before the fork.

### The trade

A version 7 UUID is no longer opaque: it discloses when it was created, to the
millisecond. Everywhere gosk stores one, it sits next to a `time` column
holding that same information, so nothing is disclosed that was not already
there.

In one place this is strictly an improvement. `writer/signalk_ws.go` was
generating its websocket session names with `uuid.NewUUID()` — **version 1**,
which embeds the host's MAC address. That is now version 7 too.

---

## 7. Can the time column go, now that the UUID carries a timestamp?

Asked once the UUIDs became time-ordered: drop `time` from `raw_data` and
`mapped_data` and derive it from the UUID instead. The answer is no.

Asked again once §6 moved to RFC 9562 method 3 and the UUIDs became accurate
to the microsecond, which removes §7.3 below. The answer is still no, for
reasons the precision never touched — §7.9 revisits it.

### 7.1 mapped_data.time is not the UUID's creation time

The UUID is the *raw message's*, generated in `message.NewRaw` when the bytes
arrived. The `time` column is the measurement's own timestamp, and several
mappers set it independently of arrival:

- `mapper/json.go` — a configured `TimestampExpression` parses the timestamp
  **out of the payload**, accepted as far as a year from arrival.
- `mapper/fft.go` — the start of the FFT window.
- `mapper/aggregate.go` — the most recent of the source values it folded in.

The two columns answer different questions: *when did this message arrive*
against *when is this measurement for*. Deriving one from the other silently
rewrites history for exactly the mappers where the difference is the point.

**Since changed.** `timestampExpression` stays — a source that carries its own
clock should be recorded at the time it reports, not the time its bytes reached
us. What changed is that the UUID now follows the timestamp instead of
contradicting it:

- `json.go` still takes the timestamp from the payload, and when it does, the
  row's UUID becomes a version 7 UUID whose embedded time *is* that timestamp
  (`mapper.uuidV7At`). So for these rows the two agree rather than differing by
  however far the sensor's clock is from ours. The derivation keeps the raw
  message's own random bits, which makes it deterministic: the same message and
  the same payload timestamp always give the same UUID, so re-processing cannot
  quietly change the identity of a row the transfer protocol counts by UUID.
- The sanity check is now symmetric, `maxTimestampSkew` either side of arrival.
  It used to compare arrival-minus-payload against +365 days, which rejects a
  payload timestamp a year in the past but accepts one arbitrarily far in the
  *future*, since that difference is negative and every negative number is
  below a positive threshold. One bad reading could put a row in a chunk years
  ahead, out of reach of the retention policy for that much longer. Arrival is
  the reference rather than `time.Now()`, so replaying stored raw data is
  checked against when it was collected.
- A rejected timestamp costs only the timestamp; the reading is still mapped,
  at the arrival time, with the raw message's UUID.
- `fft.go` stamps a spectrum with the newest sample in its window rather than
  the oldest, so it is dated by the data that completed it instead of a full
  window into the past. Pinned by a test.
- `aggregate.go` needed no change: it takes the maximum of its *inputs'*
  timestamps, so it follows whatever they are.

`notification.go`, `connector_status.go` and `meteohydro.go` still use
`time.Now()` — they report events with no upstream message, so there is nothing
for them to inherit.

A latent panic turned up while testing this. The timestamp path asserted
`output.(string)` without the comma-ok form, so a payload that simply does not
carry the field the expression reads — the expression evaluates to nil — panicked
and took the mapper process down over one malformed message. It is now checked
and logged.

Note what this does and does not buy. For json-mapped rows the UUID and the
timestamp now agree. It still does **not** make the timestamp recoverable from
the UUID in general, for the reason in §7.1.1: FFT, notification,
connector-status and meteohydro rows carry no UUID at all.

### 7.1.1 And a whole class of rows carries no UUID at all

Stronger than the above, and on its own enough to settle the question:
`message.NewSource()` returns `&Source{}`, so `Uuid` starts as the zero value,
and these mappers never set one —

- `mapper/fft.go` — sets `uuid.Nil` explicitly and never replaces it
- `mapper/notification.go`
- `mapper/connector_status.go`
- `mapper/meteohydro.go`

— so their rows reach `mapped_data` with `uuid.Nil`. That is by design, not by
accident: `20220214103722` declares the column
`uuid UUID NOT NULL DEFAULT uuid_nil()`, i.e. "this row came from no raw
message".

`uuid.Nil` is version 0, so `uuid_timestamp()` returns NULL for it. For FFT
spectra, notifications, connector-status reports and meteohydro rows there is
therefore no embedded timestamp to fall back on. Dropping `time` would not give
those rows a less precise timestamp; it would leave them with none.

This also bounds what changing the mappers could achieve. Making every mapper
stamp rows with arrival time would align §7.1's three cases, and still leave
this class with no UUID to read a time from — so `time` could not be dropped
afterwards either.

### 7.2 The UUID is not unique per mapped row

One raw message produces many mapped rows — which is precisely what
`SelectCountPerUuid`'s `GROUP BY uuid` counts, and what the whole data-request
path is built on. A column that repeats across rows cannot carry a per-row
timestamp even in principle.

### 7.3 Millisecond against microsecond — *since resolved, see §7.9*

A version 7 UUID embeds milliseconds; `timestamptz` stores microseconds.
`message/raw.go`'s own comment describes "every 2kHz sample from the shaft
power meter" — a sample every 0.5 ms. Two UUIDs generated back to back
extract to the identical timestamp (verified: both
`2026-09-24 16:30:20.467`). With `time` inside the unique index
`(time, origin, context, connector, path)`, two such samples would collide and
upsert over one another.

google/uuid does hide sub-millisecond information in `rand_a`, but that is an
implementation detail rather than part of the format, and it is perturbed
whenever the monotonic counter has to be bumped. It is not a clock.

That is what §6 fixed by moving to method 3, which writes that field on the
scale the readers expect. This objection no longer stands.

### 7.4 time is the partitioning column — a cost, not a blocker

This objection was first written as "TimescaleDB would need a
`time_partitioning_func` over the UUID". That is out of date. Verified on
2.27.1: a hypertable partitions on a `uuid` column directly —

```sql
SELECT create_hypertable('uuid_part', by_range('id', INTERVAL '1 day'));
```

— and `time_bucket(INTERVAL '1 hour', <uuid column>)` works and returns a
`timestamptz`, so a continuous aggregate over a UUID-partitioned hypertable is
possible too. (`time_bucket` on a UUID was missing as recently as October 2025,
timescaledb issue #8781; it has since landed.)

So this is a migration cost rather than an impossibility: every hypertable
recreated, every retention policy, reorder policy and continuous aggregate
rewritten, `transfer_local_data` rebuilt, plus the transfer period logic,
Grafana and the SignalK reads. Large, but doable — which is why the decision
rests on §7.1 to §7.3 and §7.5, not on this.

### 7.5 The storage argument runs the other way

It would trade away 8 bytes of `timestamptz` while keeping 16 bytes of UUID.
And once the columnstore work lands — the thing that motivates version 7 in
the first place — an ordered timestamp column is the *most* compressible
column in these tables, since delta-delta encoding is built for exactly that
shape, while a UUID's low 74 bits are random and compress poorly. This would
drop the cheapest column to keep the most expensive one.

### 7.6 Converting the existing version 4 UUIDs: don't

It would not be a conversion. Version 4 holds no timestamp, so every row would
need a *newly generated* UUID derived from its `time` — a re-keying, not a
rewrite of the same fact.

**It would break the transfer protocol.** The UUID is a correlation key
between two systems: `SelectCountPerUuid` compares per-UUID counts between
vessel and cloud over arbitrary historical periods. Re-key one side and every
period reads as incomplete forever, triggering wholesale retransmission.
Avoiding that means re-keying every vessel and the cloud identically and at
the same moment, including vessels that have been offline for weeks.

**It would buy nothing.** Version 7's benefit is index locality *at insert
time*. Rewriting rows that were inserted long ago does not change how future
rows insert.

**It would cost a full rewrite.** An `UPDATE` of every row means the table
rewritten, indexes rebuilt, WAL proportional to the whole history, and
decompression and recompression of any compressed chunk — across a
`mapped_data` that has no retention policy at all.

What to do instead: `raw_data` heals itself inside its 7-day retention. For
`mapped_data`, if the real worry is fragmented old chunks, the
`add_reorder_policy` already on both tables reorders them physically by index
and touches no data. And if a time-ordered surrogate key is ever genuinely
wanted, add a column rather than rewriting `uuid`.

### 7.7 Extracting the timestamp in SQL

Three implementations exist, and the one in the middle has a trap in it.

**PostgreSQL.** `uuid_extract_timestamp` arrived in 17, but that version
handles version 1 only:

| Server | `uuid_extract_timestamp` | on a version 7 UUID |
| --- | --- | --- |
| 16 and earlier | absent | — |
| 17 | present | **returns NULL** (handles version 1 only) |
| 18 | present | works |

PostgreSQL 17 is the trap: the function exists, `uuid_extract_version`
correctly answers `7`, and extraction silently returns NULL. Verified on a
17.11 server where a version 1 UUID returns `2022-02-22 20:22:22+01` while
every version 7 UUID returns NULL.

**TimescaleDB** added its own set in 2.23, in the `public` schema —
`generate_uuidv7()`, `to_uuidv7(timestamptz)`, `to_uuidv7_boundary(timestamptz)`,
`uuid_version(uuid)`, `uuid_timestamp(uuid)` and `uuid_timestamp_micros(uuid)`.
`uuid_timestamp` reads gosk's Go-generated UUIDs correctly and is immutable,
parallel safe and strict (verified against 2.27.1).

**Use TimescaleDB's.** gosk creates the TimescaleDB extension in its first
migration and six migrations depend on its functions, so it is always present
— which makes `public.uuid_timestamp(uuid)` the one to reach for, with no
wrapper and nothing to install:

```sql
SELECT time, uuid, uuid_timestamp(uuid) AS created
FROM mapped_data
WHERE origin = '…' AND time BETWEEN … AND …;
```

An earlier draft of this branch added a `public.uuid_v7_timestamp(uuid)` that
picked between the three implementations at migration time. It was deleted: a
debugging aid does not justify a permanent database object, a migration that
can never leave the sequence, and a second function named almost exactly like
TimescaleDB's.

Check what a server has with:

```sql
SELECT current_setting('server_version') AS pg,
       (SELECT extversion FROM pg_extension WHERE extname = 'timescaledb') AS timescaledb;
```

On TimescaleDB older than 2.23, where `uuid_timestamp` does not exist yet,
this is the same answer inline — no DDL, correct for gosk's UUIDs, and NULL
for any other version:

```sql
CASE WHEN substr(uuid::text, 15, 1) = '7' THEN
    to_timestamp(
        ('x' || lpad(substr(uuid::text, 1, 8) || substr(uuid::text, 10, 4), 16, '0'))
            ::bit(64)::bigint / 1000.0
    )
END
```

All three — TimescaleDB 2.27.1, PostgreSQL 18.6, and the expression above on
17.11 — were checked against the same UUIDs and agree to the millisecond,
returning NULL for versions 1 and 4 and for the nil UUID.

### 7.8 Do not use `uuid_timestamp_micros` on gosk's UUIDs

TimescaleDB's `uuid_timestamp_micros` looks like the answer to §7.3's
resolution problem. It is not, for two independent reasons.

**The sub-millisecond encoding does not match.** Both implementations keep a
sub-millisecond fraction in `rand_a`, but they scale it differently:
TimescaleDB writes and reads 1/4096ths of a millisecond, while google/uuid —
which generates every UUID gosk stores — writes units of 256 ns. So the field
is read back against the wrong scale. Measured: a UUID whose real fraction is
125 µs comes back as `.455119` rather than `.455125`, roughly 4.9% low, up to
about 46 µs at the top of a millisecond. The error is silent, and it is not a
TimescaleDB bug — its own `to_uuidv7` round-trips through
`uuid_timestamp_micros` exactly. The two formats simply disagree about a field
the specification leaves implementation-defined.

**The fraction is not a clock anyway.** google/uuid bumps that same field to
keep values monotonic, so for two UUIDs generated inside one 256 ns window it
holds a counter, not a measurement. The burst pair in §7.3 shows it directly:
`…7bda` and `…7bdb` report `.46774` and `.467741`, one microsecond apart — an
artefact of the counter increment, not of anything that was timed.

Milliseconds are the honest resolution. Use `uuid_timestamp`, not
`uuid_timestamp_micros`.

None of this is something gosk queries — it is for reading the database by
hand. Comparing a row's `uuid_timestamp(uuid)` against its `time` shows the gap
between arrival and measurement described in §7.1, which is exactly the sort of
discrepancy the count-based reconciliation in §3 cannot see.

Two properties worth knowing while using it. Resolution is milliseconds, so
UUIDs from the same millisecond report the same time. And PostgreSQL's
bytewise UUID ordering matches time ordering for version 7, so a range
predicate on the UUID column is valid — TimescaleDB's
`to_uuidv7_boundary(timestamptz)` builds the bounds for one, zeroing the random
bits:

```sql
WHERE uuid >= to_uuidv7_boundary('2026-09-24 10:00+02')
  AND uuid <  to_uuidv7_boundary('2026-09-24 11:00+02')
```

No chunk pruning comes with that, though, since these chunks are partitioned on
`time` — so keep filtering on `time` as well, as §7.4 explains.

### 7.9 Revisited, with microsecond-accurate UUIDs

§6's move to RFC 9562 method 3 settles §7.3: a UUID now carries the time to
about 244ns, finer than the microsecond `timestamptz` itself stores, and a
row written through gosk's own database layer comes back with its `time`
column and the timestamp extracted from its UUID agreeing exactly. Two
samples 500µs apart — the 2kHz case that prompted the question — are 2048
steps apart in `rand_a`, nowhere near colliding.

So the precision objection is gone. The column still cannot go, for four
reasons the precision never addressed.

**Rows already written cannot be reconstructed, and `mapped_data` never
expires.** Dropping the column requires *every* row's timestamp to be
recoverable from its UUID, and three populations fail that:

- rows from before §6, carrying version 4 UUIDs, which hold no time at all;
- rows written between §6 and the move to the fork, carrying version 7 UUIDs
  whose sub-millisecond field is on google/uuid's scale — read back by
  `uuid_timestamp_micros` about 5% low, see §7.8;
- rows carrying `uuid.Nil`, below.

`raw_data` clears itself inside its 7-day retention. `mapped_data` has **no
retention policy at all**, so it will hold all three indefinitely. There is
no version of this that does not begin with either back-filling every
historical row — which §7.6 explains is a re-keying that would break the
transfer protocol — or accepting that rows before some cutover simply lose
their timestamps.

**Four mappers still emit `uuid.Nil`.** §7.1.1 found this and it is
unchanged: `fft.go` sets it explicitly, and `meteohydro.go`,
`notification.go` and `connector_status.go` never set a UUID at all, so
`message.NewSource()`'s zero value stands. `aggregate.go` starts from
`uuid.Nil` too but replaces it with the UUID of the value it took its
timestamp from, so it is fine. Fixing the four is small — they know their
own timestamp, so `uuid.Must(uuid.NewV7AtTimePrecise(t))` would do — but
until it is done and those rows have aged through, a share of `mapped_data`
has no time in its UUID at all.

**`message.NewRaw` reads the clock twice.** It calls `NewV7Precise()` and
`time.Now()` separately, so the UUID's embedded time is a microsecond or two
behind `Timestamp`. That did not matter while the UUID was only accurate to
a millisecond; at microsecond resolution it is a real, if tiny,
disagreement between two fields that are meant to say the same thing. Worth
fixing on its own merits: take one `time.Now()` and build the UUID from it
with `NewV7AtTimePrecise`.

**The storage argument still runs backwards**, and is now the strongest
standing reason. §7.5: this trades away 8 bytes of `timestamptz` while
keeping 16 bytes of UUID, and once the columnstore work lands an ordered
timestamp column is the *most* compressible column in these tables, where a
UUID's low bits are random and compress poorly. Dropping the cheap column to
keep the expensive one does not become a better trade because the expensive
one got more precise.

### 7.10 What it would take, if the answer ever changes

Roughly, and in this order:

1. Give the four mappers of §7.1.1 real UUIDs, and make `NewRaw` derive both
   fields from one clock read.
2. Put a retention policy on `mapped_data`, or accept a permanent cutover
   date before which rows have no recoverable timestamp.
3. Wait out that retention, so that every remaining row carries a method 3
   UUID.
4. Repartition both hypertables on the UUID column — possible, see §7.4 —
   and rewrite every retention policy, continuous aggregate,
   `transfer_local_data`, the transfer period logic, Grafana and the SignalK
   reads against it.
5. Rewrite every time range predicate as a UUID range, via
   `to_uuidv7_boundary`.

The payoff at the end of that is 8 bytes a row before compression, and close
to nothing after it. The reason to keep asking the question is not the bytes
but the appeal of one column meaning one thing — which §7.1's change to
`json.go` already delivers, without dropping anything.

### 7.11 Re-keying historical rows as a pure function — this works

Asked after §7.9: rather than back-filling with fresh randomness, rebuild
each existing UUID *from what the row already has* — its `time` and its
current UUID — with a pure function. Same inputs, same output, every time.

It works, and it removes the objection §7.6 was built on.

```sql
CREATE FUNCTION public.uuid_v7_at(ts timestamptz, base uuid) RETURNS uuid
  -- 48 bits of unix milliseconds from ts
  -- version nibble, then ts's sub-millisecond fraction in 4096ths (method 3)
  -- the variant nibble, forced
  -- the remaining 60 bits taken from base, untouched
```

Verified on TimescaleDB 2.27.1 over every microsecond of a full second
against three kinds of base — a version 4 UUID, a version 7 one, and
`uuid.Nil` — 3,000,000 reconstructions in total. Every one recovers its
exact microsecond through `uuid_timestamp_micros`, every one is version 7,
and all million per base are distinct.

**Why this defeats §7.6.** That section argued a re-key would desynchronise
the transfer protocol, because vessel and cloud must agree on UUIDs and
could not both be rewritten at once. That reasoning does not survive
purity: both sides hold the same `(time, uuid)` for a row, so both compute
the same new UUID independently, with no coordination and no message
exchanged. The objection was to re-keying with *fresh randomness*; it does
not apply here.

**Two things testing caught**, both of which would have produced quietly
wrong UUIDs:

- `(extract(epoch from ts) * 1000)::bigint` **rounds**, while the
  sub-millisecond fraction truncates. A timestamp in the last millisecond of
  a second therefore got the next millisecond in the timestamp field and the
  old fraction below it — off by a whole millisecond, for one microsecond in
  every thousand. It needs `floor`.
- The variant nibble has to be written, not inherited from `base`. Taking it
  from `uuid.Nil` yields variant `0`, which is not a valid RFC 9562 UUID at
  all.

**What it does not solve.**

*The rollout is still not atomic.* While the cloud has re-keyed and a vessel
has not, `SelectCountPerUuid` compares new UUIDs against old ones, finds no
overlap, and reads every period as entirely missing — triggering exactly the
mass retransmission §7.6 warned about. Purity gives a way out that fresh
randomness did not: the un-migrated side can apply the function at query
time, so the two agree before either has rewritten anything. That needs
designing, but it is a real path rather than a dead end.

*The rewrite is still a full rewrite.* `UPDATE` over every row of
`mapped_data`, with index rebuild and WAL to match, and decompression and
recompression of any compressed chunk — on a table with no retention policy,
so the whole history.

*The raw-to-mapped link partly breaks.* A mapped row and the raw row it came
from share a UUID today. After a re-key they share one only where they also
share a timestamp; a row whose time came from `timestampExpression`, an FFT
window or an aggregate gets a different result from its raw row. Mapped rows
from one update still group together, which is what `SelectCountPerUuid`
counts, but this wants checking against the transfer protocol before anyone
runs it.

*And §7.5 is untouched.* The compression argument does not care how the
UUIDs were produced.

**One pleasing side effect.** A re-keyed row's UUID would agree with its
`time` *exactly* — better than newly written rows manage today, since
`message.NewRaw` reads the clock twice (§7.9). Worth fixing that first, so
that new data is not the worse of the two.
