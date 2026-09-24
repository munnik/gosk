# Review: vessel → cloud data transfer

A review of the data transfer / reconciliation system (`transfer/`, the transfer
queries in `database/postgresql.go`, and the MQTT read/write path), with
recommendations.

**Short version:** the transfer system is a reconciliation loop bolted on top of
a transport that is configured to lose data. Fix the transport first — most of
the gaps the reconciliation loop chases are self-inflicted.

**Status:** §1, §2 and §6 are implemented (see "Deploying §1" below for the two
prerequisites this cannot enforce from the code). §3 and §4 are unchanged
proposals; nothing in them has been built.

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

### No new dependency, and nothing hand-rolled

`github.com/google/uuid` — already a direct dependency — has implemented
version 7 since v1.6.0 as `uuid.NewV7()`.

The call is written out at each site rather than hidden behind a helper, so
which version is in use is visible where it matters. `grep -rn 'uuid\.New()'`
returning nothing is what says the rule holds; keep it that way when adding
code.

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

### The trade

A version 7 UUID is no longer opaque: it discloses when it was created, to the
millisecond. Everywhere gosk stores one, it sits next to a `time` column
holding that same information, so nothing is disclosed that was not already
there.

In one place this is strictly an improvement. `writer/signalk_ws.go` was
generating its websocket session names with `uuid.NewUUID()` — **version 1**,
which embeds the host's MAC address. That is now version 7 too.
