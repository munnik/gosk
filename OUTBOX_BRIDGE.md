# The outbox beside the count protocol

This branch (`transfer-outbox-bridge`) is `master` plus the transfer outbox,
with nothing taken away. It exists so that a fleet can run both transfer
protocols at once, towards two servers:

- `gosk transfer respond` on a vessel keeps answering the central server's
  count protocol (`gosk transfer request`), as it always has;
- `gosk transfer send` on the same vessel, from the same database, delivers
  each minute of mapped data to a second server running `gosk transfer
  receive`, which stores and acknowledges it.

What it carries from the `transfer-outbox` branch: `database/outbox.go`,
`transfer/{outbox,send,receive,seed,digest,digest_respond,validate}.go`, the
`outbox` and `external_migrations` options of the database configuration,
the MQTT client's persistent sessions and subscription checks, the writer's
retry backoff and the migration retry. The three migrations that create
`transfer_outbox`, `transfer_received` and the epoch tables are on `master`
as well, see "Going back".

What it leaves out, on purpose: the migrations that drop the `transfer_*`
tables and the `uuid` columns, the removal of the uuid from every message,
and the columnstore migrations. The old protocol needs the first two, and
the third is a decision of its own (`DEPLOY_OUTBOX.md`, step 1).

## What running both means

- The vessel's mapped writer does the outbox bookkeeping (`outbox: true`),
  so every gosk process on the vessel has to be this build. The message
  format is unchanged from `master`, so a mixed pipeline still works, but
  only this build's writer marks buckets.
- The two protocols share nothing but the database. The responder reads
  `mapped_data` by uuid, the sender by time; neither writes what the other
  reads.
- Only one server may acknowledge. An acknowledgement lets the vessel forget
  the minute, so a second acknowledging receiver would split the fleet's
  data between the two servers. The server that runs the count protocol
  runs no receiver.
- The MQTT client keeps a persistent session, under a stable client id, only
  for the outbox roles (`mqtt.sessionRoles`). A vessel runs two `write mqtt`
  processes against the same broker with the same username, and a derived
  client id for those would have them disconnecting each other.
- Migration numbers: this branch adds no migration of its own any more -
  the three it needs are on `master` (see "Going back"). The newest is
  `20261020120000`. When the full `transfer-outbox` branch is deployed
  later, its drop migrations (`20261017120100`, `20261017120200`) sort
  below that and golang-migrate would skip them on a database that ran
  this. Renumber them above `20261020120000` when that day comes.

## Going back

`master` carries the same three migrations since "The outbox tables, ahead
of the outbox" (its PR #30), and has to be deployed before this branch is:
golang-migrate refuses to start from a schema version the binary has no
migration for, and a deploy that is rolled back to an older build - which
deploy-rs does on its own when an activation fails - would otherwise leave
every gosk process refusing the database. With the tables on both sides,
the two builds replace each other freely, and going back is deploying
`master` again: `transfer_outbox` simply stops being written.

Only a `master` build from before PR #30 does not know the version, and
only for one of those does the database need its version set back first,
in the gosk user's schema:

```sql
UPDATE schema_migrations SET version = 20261015120000, dirty = false;
```

Set it forward again (`version = 20261020120000`) before deploying any
build that has the three migrations - this branch or a `master` from PR
#30 on - or that build runs them a second time, fails on the sequence that
already exists, and leaves the schema marked dirty.
