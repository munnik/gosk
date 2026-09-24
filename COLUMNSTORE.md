# Columnstore compression for mapped_data

What the `timescale-columnstore` branch does, what it changes in the database,
and what it means for queries.

All figures here were measured on a nightly clone of production, not estimated.

## Why

The gosk database had reached ~29TB logical (~17.6TB on disk after ZFS zstd).
Nothing in it was ever compressed by TimescaleDB. Two separate problems:

| table | size | problem |
| --- | --- | --- |
| `transfer_log` | 18TB | no retention policy, and 96% of its bytes were one logged field |
| `mapped_data_matching_context` | 3678GB | uncompressed; 1981GB of that is index, more than the 1586GB heap |
| `mapped_data_other_context` | 7341GB | uncompressed; 3244GB index against 4096GB heap |
| `mapped_data_5min_hidden` | 699GB | uncompressed continuous aggregate |
| `mapped_data_1hour_hidden` | 108GB | uncompressed continuous aggregate |

On the `mapped_data` tables the btree indexes are as large as the data they
point at. A chunk in the columnstore has no per-chunk btrees at all, so that
half of the table disappears on top of the compression of the heap itself.

## What the branch changes

Five commits, four of them migrations:

1. **`transfer/main.go`, `request.go`, `respond.go`** - stop logging the full
   `counts_per_uuid` map to `transfer_log`. Application change, no schema
   impact. The MQTT protocol is untouched; only what is written to the log
   table shrinks.
2. **`20260924150000_transfer_log_retention`** - 1-day chunks and a 30-day
   retention policy on `transfer_log`.
3. **`20260924150500_disable_bulk_decompression`** - a database-level setting
   that works around a TimescaleDB crash. Must run before the next two.
4. **`20260924151000_columnstore_mapped_data`** - columnstore on both
   `mapped_data` hypertables, after 60 days.
5. **`20260924152000_columnstore_continuous_aggregates`** - columnstore on the
   two continuous aggregates, after 200 days. Guarded, so it is a no-op on a
   vessel node where those aggregates do not exist.

## Database structure changes

### No schema changes

No table is created, dropped or altered. No column is added, removed or
retyped. No view or continuous aggregate definition changes. Every query that
works today still compiles and returns the same rows.

What changes is how chunks are *stored*, and which background jobs exist.

### Storage

Chunks older than the policy threshold are rewritten into the columnstore.
This is per-chunk and gradual - the policy converts eligible chunks on its
schedule (every 12 hours), it does not rewrite the table at once.

```sql
ALTER TABLE "mapped_data_matching_context" SET (
    timescaledb.enable_columnstore,
    timescaledb.segmentby = 'origin',
    timescaledb.orderby = 'time DESC'
);
```

Measured on a real 35GB chunk:

| | before | after |
| --- | --- | --- |
| total | 35GB | 1517MB (23.4x) |
| indexes | 19GB | 5752kB |

### Indexes

A chunk's btree indexes are dropped when it is converted, and replaced by
sparse min/max indexes derived from `orderby`. The index *definitions* on the
hypertable stay - new, uncompressed chunks still get all four:

- `mapped_data_unique_idx (time, origin, context, path, connector)` - still
  enforced on columnstore chunks, see "Upserts" below
- `mapped_data_origin_path_time_idx (hashtext(origin), hashtext(path), time)`
- `mapped_data_origin_uuid_time_idx (origin, uuid, time)`
- `mapped_data_time_idx (time DESC)`

### Background jobs

Added:

| job | target | setting |
| --- | --- | --- |
| `policy_compression` | `mapped_data_matching_context` | after 60 days |
| `policy_compression` | `mapped_data_other_context` | after 60 days |
| `policy_compression` | `mapped_data_5min_hidden` | after 200 days |
| `policy_compression` | `mapped_data_1hour_hidden` | after 200 days |
| `policy_retention` | `transfer_log` | drop after 30 days |

Removed: the two `policy_reorder` jobs added in `20230423230935`. TimescaleDB
refuses a reorder policy on a compressed hypertable, and `orderby` does the job
reorder was there for.

### Database setting

```sql
ALTER DATABASE gosk SET timescaledb.enable_bulk_decompression = off;
```

See "The crash" below. It applies to connections opened after the migration
runs, which is why it is ordered ahead of the columnstore migrations.

### Chunk intervals

`transfer_log` goes from 7-day to 1-day chunks, so the 30-day retention does
not silently become 37 days - `drop_chunks` only removes a chunk once its whole
range has expired. Same fix as `raw_data` in `20260915180000`.

`mapped_data` chunk intervals are **left at 7 days** deliberately. Narrower
chunks would give the node disk-pressure trim finer granularity, but they would
also multiply the chunk count on a table with four years of history, and
production already runs thousands of chunks with `max_locks_per_transaction`
raised to 1024.

## Query impact

### Unchanged

Row counts and results are identical. Verified on every query shape below - the
same 2387, 251 and 10253 rows before and after conversion.

### Faster

Cold-cache numbers matter most here: the database is far larger than RAM, so
most production reads miss cache. Measured on a real 35GB chunk:

| query shape | rowstore cold | rowstore warm | columnstore cold | columnstore warm |
| --- | --- | --- | --- | --- |
| transfer reconciliation (origin + 5 min, group by uuid) | 1298ms | 8.23ms | 8.15ms | 2.27ms |
| `get_mapped_data` (context + path, 24h) | 9732ms | 936ms | 103ms | 55.3ms |
| continuous aggregate refresh shape (1h) | 26850ms | 339ms | 277ms | 191ms |

Roughly 100x cold, 2-17x warm. The gain comes from reading ~23x less data and
from column projection - a query touching three columns no longer reads the
other six.

### Slower

**Upserts into a compressed chunk.** gosk's `mappedInsertQuery` is an
`ON CONFLICT ... DO UPDATE`, and when the target rows are in the columnstore
TimescaleDB has to decompress the batches holding them. Re-sending 6527 rows of
a two-minute period took 948ms against 409ms for the same upsert into an
uncompressed chunk - about 2.3x.

This only applies to data older than the 60-day threshold. Sampling what
transfer was actually retrying: 88% of requests were for the current month and
6% for the month before, with a thin tail reaching back a year. So routine
catch-up still lands in the rowstore.

**The `segmentby`/`orderby` choice is what makes this survivable.** Three
configurations were measured:

| configuration | ratio | 6527-row backfill upsert |
| --- | --- | --- |
| `segmentby=origin, orderby=time DESC` | 20.96x | 948ms |
| `segmentby=origin, orderby=path,time DESC` | 16.83x | fails |
| `segmentby=origin,context,connector, orderby=path,time DESC` | 16.82x | fails |

Both `path`-first orderings fail outright with
`ERROR: tuple decompression limit exceeded by operation`: sorting batches by
path scatters any given time range across the whole chunk, so a small backfill
has to decompress past `max_tuples_decompressed_per_dml_transaction` (100000)
to find its conflicting rows. An ordering that cannot absorb a backfill is not
usable here whatever it compresses to.

### Broken: `CREATE TABLE ... AS SELECT`

TimescaleDB segfaults on `CREATE TABLE ... AS SELECT` over a columnstore chunk.
It is SIGSEGV, so postgres terminates every other session and enters crash
recovery - a whole-database outage, not a failed query.

Reported upstream as
[timescale/timescaledb#10655](https://github.com/timescale/timescaledb/issues/10655).
Still present in 2.30.1, the current release, so waiting for an upgrade is not
a plan.

`20260924150500_disable_bulk_decompression` works around it. With
`timescaledb.enable_bulk_decompression = off` the same statement completes and
returns the correct rows. Measured cost of that setting: ~7-8% on the
reconciliation and aggregate-refresh shapes, against ~23x less data to read.

What was verified *not* to crash, on the same compressed chunk:

- plain `SELECT *` returning rows to a client (Grafana, gosk)
- `SELECT count(*)` over a whole chunk
- `INSERT INTO <table> SELECT * FROM <columnstore chunk>`
- the aggregate shape a continuous aggregate refresh uses

So gosk, Grafana and the continuous aggregate refresh were never at risk. The
exposure was ad-hoc `CREATE TABLE x AS SELECT ...`, which nine roles could run
- including `jupyter` and seven analyst accounts.

**Do not** reach for `timescaledb.enable_columnarscan = off` instead. That is a
separate and worse bug
([timescale/timescaledb#10656](https://github.com/timescale/timescaledb/issues/10656)):
with it set, both `SELECT` and CTAS return *zero* rows from a compressed chunk,
silently and with no error.

## transfer_log

Handled by retention, not compression, because compressing it does nothing: a
30-minute slice (1020833 rows, 2489MB) converted to the columnstore in two
minutes and came out at 2312MB - a ratio of 1.08. Its bytes are already-TOASTed
jsonb holding random uuid keys, so there is no entropy left to remove.

Its growth was a step change rather than a trend: ~60GB/month through 2023 and
2024, then ~2400GB/month from 2026-03, so 15.3TB of the 18TB was written in the
last seven months. The `LoggedRequestMessage` change removes the cause and the
retention policy clears what accumulated.

Nothing in gosk reads the table - `LogTransferRequest` is the only statement
that touches it - but nine roles hold `SELECT` on it, including `jupyter` and
seven analyst accounts. Worth one check that no notebook wants more than a
month of history before this ships.

## Expected result

If `mapped_data_other_context` behaves like `matching_context` did, the
`mapped_data` tables and their aggregates go from ~11.8TB to roughly 1.2TB (the
60-day threshold leaves ~10% uncompressed). With `transfer_log` bounded, the
database should land somewhere near 1.5TB logical against today's ~29TB.

## Rollout notes

- Compression is gradual. Each policy run converts eligible chunks; it does not
  rewrite the table in one pass. Converting a 35GB chunk took a few minutes and
  is CPU-bound.
- The migrations are ordered so the crash workaround is in place before any
  chunk is compressed. Do not apply them out of order.
- The continuous aggregate migration is a no-op on vessel nodes.

## Rollback

All four migrations have `down` counterparts, verified to run cleanly through
golang-migrate: policies removed, compression settings cleared, database
setting reset, reorder policies restored.

Two caveats:

- Rolling back the columnstore migration decompresses every converted chunk.
  On a database this size that is slow and needs the disk space back.
- Rolling back the `transfer_log` retention removes the policy, but the data it
  already dropped is gone.
