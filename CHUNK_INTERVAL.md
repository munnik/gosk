# Daily chunks for mapped_data

What the `timescale-chunk-interval` branch does, the live measurements behind
it, and what the same review found that already lives on other branches.

All measurements were taken on 2026-10-17 on the-test-server's copy of the
production database (PostgreSQL 17.11, TimescaleDB 2.27.1, N GB), read-only.

## The branch

One migration, `20261018100000_daily_mapped_data_chunks`: 1-day chunks on
`mapped_data_matching_context` and `mapped_data_other_context`, in place of
the 7-day default both were created with. It affects chunks created from then
on; existing 7-day chunks keep their width until a node trims them or a
server converts them to the columnstore.

It is branched from `timescale-columnstore`, which `transfer-outbox` already
contains, so it merges into either. The migration is independent of both:
`set_chunk_time_interval` is a catalog update and works the same with or
without the columnstore. The database suite passes with it, up and down.

## Live state

| table | size | chunks | interval | per day |
| --- | --- | --- | --- | --- |
| `mapped_data_other_context` | N GB (N GB index) | 138 | 7 days | 28M rows, ~N GB |
| `mapped_data_matching_context` | N GB (N GB index) | 138 | 7 days | 14M rows, ~N GB |
| `transfer_log` | N GB (N GB TOAST) | 6 | 1 day since PR #26 | ~N GB |
| `mapped_data_5min_hidden` (nix repo) | N GB | 26 | 70 days | |
| `mapped_data_1hour_hidden` (nix repo) | N GB | 26 | 70 days | |

Nothing is compressed on live. There is no retention on the mapped tables;
their history starts 2024-02-29, so older chunks were dropped by hand once.
The reorder policies from `20230423230935` and `20231024113638` are not on
live either, which `timescale-columnstore` already makes official.

One 7-day chunk, with its indexes:

| | heap | index | rows |
| --- | --- | --- | --- |
| `mapped_data_other_context` | 36-N GB | 33-N GB | ~200M |
| `mapped_data_matching_context` | 17-N GB | 26-N GB | ~100M |

## Why one day

**Vessel nodes lose data a chunk at a time.** `zfs-ensure-free-space` in the
nix module (`modules/nixos/containers/postgresql-gosk.nix`) drops the oldest
`mapped_data` or `raw_data` chunk every six hours while the pool is over 75%.
A 7-day chunk makes that a week of the vessel's own history per run; on
a-vessel one drop moved N GB into a snapshot at once.
`raw_data` went to daily chunks for this reason in `20260915180000`;
`mapped_data`, the copy that is actually read back, still loses a week at a
time.

**The server's active chunk pair is ~N GB.** the-production-server has N GB of
RAM and N GB of `shared_buffers`. TimescaleDB's sizing guidance is for every
actively written chunk, indexes included, to fit in roughly that quarter. A
daily pair is ~N GB.

**Columnstore conversion gets a week of slack and ~N GB of headroom back.**
The policy on `timescale-columnstore` converts a chunk once its whole range is
past 60 days, and the compressed copy sits next to the uncompressed one until
the conversion commits. With daily chunks that is a day of slack and ~N GB.

**Outbox digests are per day.** `transfer-outbox` summarises `mapped_data`
per day, hour and minute on the time column. A day-level digest then reads
exactly one chunk, not a seventh of one through the index.

## The chunk-count argument

`COLUMNSTORE.md` kept 7 days because of chunk count and the raised
`max_locks_per_transaction`. The numbers:

- The server holds 345 chunks today, 1596 relations under
  `_timescaledb_internal`. At a day each, 2.6 years of history is about 1900
  `mapped_data` chunks; nodes trim, so they stay far below that.
- The lock table is `max_locks_per_transaction x max_connections`: 1024 x 500
  on a server, 256 x 100 on a node. A statement over the whole `mapped_data`
  view takes about four locks per rowstore chunk (heap plus three indexes
  once the uuid index is gone) and two or three per columnstore chunk, so
  ~6000 for the worst case against 512 000 slots.
- Planning cost grows with chunks that survive exclusion, not with chunks
  that exist. Every gosk query carries a time predicate, and the aggregates
  Grafana reads keep their 70-day chunks.

## What else the review found, and where it already lives

| finding | state |
| --- | --- |
| No compression; indexes as large as the heap | `timescale-columnstore`: measured 23x on a real chunk, `segmentby origin, orderby time DESC`. Ordering by path first compresses slightly better but fails on backfill upserts with `tuple decompression limit exceeded`, so do not "improve" the orderby from the index shape. |
| `transfer_log` writes ~N GB/day, 96% of it the `counts_per_uuid` map in data requests (avg 94 kB, max 378 kB) | `timescale-columnstore` logs the map's size instead; master has the 30-day retention; `transfer-outbox` drops the table. |
| Backfill older than 180 days can never complete: the `transfer_local_data` aggregates are materialized-only and refresh 180 days, so the requester re-asks forever. 60k of the 106k incomplete periods are older than that; the median data request is for a period 82 days old, the 90th percentile 382 days. | Moot once `transfer-outbox` replaces the requester. Until then it is wasted traffic and dead tuples in old chunks. |
| `uuid` is a random 16 bytes per row and over half the compressed bytes | `transfer-outbox` drops it, plus the (origin, uuid, time) index (N GB). |

## Still open, outside this branch

- **nix repo, the-production-server migrations:** `mapped_data_5min_hidden`
  refreshes 180 days every 5 minutes under a 4-minute cap, and in the month
  of job history test keeps (from 2026-09-29) every one of its 1010 runs
  hit that cap and failed; the hourly aggregate's job failed once at its
  5-hour cap on 2026-10-19. Yet the 5-minute aggregate's newest bucket was
  2026-10-17 06:50 UTC on a 07:05 snapshot, so something still advances it
  (partial commits, a manual refresh); how complete the 180-day window is
  behind that is unknown. The fix is two windows: a short frequent one for
  recent data and a slower, non-overlapping one for late data, sized to the
  lateness that actually occurs. Smaller chunks do not fix this. While in
  there: that aggregate's materialized hypertable uses 70-day chunks of up
  to 113 GiB; 7-day chunks for new ones would make its compression and any
  retention far cheaper. The hourly one (max 18 GiB per chunk) can stay.
- **`transfer-outbox`:** acknowledged buckets are never deleted. That is
  ~27k rows a day on the server (19 origins x 1440 minutes) and 1440 on a
  node: small, but a periodic delete of fully acknowledged buckets older
  than a few days keeps the table and its two indexes from growing without
  bound.
- **Duplicate `transfer_log` migrations:** `20260924150000` on
  `timescale-columnstore` and `20261015120000` on master set the same
  interval and policy. The outbox worktree already made the master one
  `if_not_exists`; the remaining double `set_chunk_time_interval` is
  harmless.
- **Retention on the server's mapped tables** is a choice nobody has made
  yet. With the columnstore in place it is no longer a question of space.

## Rollout

Deploying the version that carries the migration is the whole rollout. The
first chunk created after the restart is one day wide. On a node the trim
keeps dropping 7-day chunks until the old ones are gone, then drops days.
