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
retry backoff and the migration retry, and the three migrations that create
`transfer_outbox`, `transfer_received` and the epoch tables.

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
- Migration numbers: this branch's newest is `20261020120000`. When the full
  `transfer-outbox` branch is deployed later, its drop migrations
  (`20261017120100`, `20261017120200`) sort below that and golang-migrate
  would skip them on a database that ran this branch. Renumber them above
  `20261020120000` when that day comes.

## Going back

Deploying `master` again against a database that ran this branch leaves
three unused tables and a `schema_migrations` version `master` does not
know; nothing else. `master`'s `UpgradeDatabase` reports no change for a
database that is ahead of it.
