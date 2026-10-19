# Deploying the outbox and dropping the UUIDs

This version of gosk replaces the count-based transfer protocol
(`gosk transfer request` / `respond`) with an outbox (`gosk transfer send` /
`receive`, see `transfer/outbox.go`), and drops every uuid column together with
every table the old protocol kept. The order below matters: a step that is
done too early either loses live data for a while or makes a vessel wait on a
server that cannot answer it. Nothing in the order loses stored data if the
order is kept.

What the migrations do the moment the first gosk process of the new version
starts against a database (every gosk process runs them, servers and vessels
alike):

| migration | effect |
| --- | --- |
| `20261017120000_transfer_outbox` | creates `transfer_outbox` |
| `20261017120100_drop_transfer_requests` | drops `transfer_data`, `transfer_local_data` and its two continuous aggregates, `transfer_remote_data`, `transfer_log` |
| `20261017120200_drop_uuid_columns` | drops `uuid` and `transfer_uuid` from both `mapped_data` tables and `uuid` from `raw_data`, recreates the `mapped_data` view |

`DROP COLUMN` is a catalog change: measured on TimescaleDB 2.27.1 it took
0.7 ms on three compressed chunks holding a million rows, and the rows stayed
readable. The space comes back as chunks are recompressed or expire, not at
once. The drops are irreversible in content: the down migrations recreate the
structures empty, and there is no `gosk` command that runs them. The only
rollback of a database is the ZFS snapshot taken in step 2.

## 0. Before anything: the order in one view

1. Build and test the new gosk (step 1).
2. Snapshot the production database (step 2).
3. Change the broker ACLs and deploy them, on both brokers (step 3).
4. Deploy `the-production-server` with receivers and digest responders, by hand from
   a branch of the nix repo (step 4). The old reader stays, so vessels that
   are not converted yet keep arriving.
5. Deploy `the-test-server` (step 5).
6. Convert one vessel by hand and watch it (step 6).
7. Merge the nix branch to `main`; the unattended morning run converts the
   rest of the fleet (step 7).
8. Seed each vessel once, oldest gaps first (step 8).
9. Clean up what the old protocol left behind (step 9).

Do not merge the nix branch to `main` before step 4 is done and verified:
`the-office-server` deploys every node from `origin/main` at 06:30, and a vessel on
the new version sends to `outbox/data/<origin>`, which nothing stores until
the receiver on `the-production-server` runs. Nothing is lost - the vessel's outbox
keeps every minute until it is acknowledged - but the vessel's live data stops
arriving in the cloud until then.

Also check that the the-office-server backup re-seed that started on 2026-10-16 has
finished and its ZFS hold on `the-production-server` has been released before step 4
restarts services there: the schema change is replicated to both backup
copies by the next syncoid run, which is what you want, but not in the middle
of a seed.

## 1. Build and test gosk

From this branch (`transfer-outbox`, which already contains
`timescale-columnstore`):

```sh
cd ~/Code/gosk
go build ./... && go vet ./...
go test ./message/ ./transfer/ ./mqtt/ ./writer/ ./reader/ ./connector/ ./nanomsg/
nix flake check          # runs the database suite against a throwaway TimescaleDB
nix build .#gosk
```

Several `mapper` specs fail on `master` as well (stale fixtures, and a test
that reads a gitignored config file); they are not part of this change and
fail identically with it.

Once merged, update the vendor hash in `flake.nix` if `go.mod` changed (it
did not for this change) and tag or note the commit the nix repo will pin.

## 2. Snapshot the production database

On `the-production-server`, before any service of the new version starts:

```sh
ssh the-production-server
sudo zfs snapshot tank/data/postgresql-17-gosk@before-outbox-$(date +%F)
sudo zfs list -t snapshot -r tank/data/postgresql-17-gosk | tail -3
```

A snapshot of a running PostgreSQL is crash-consistent, which is enough for a
rollback. Keep it until step 9. Rolling back is: stop
`container@postgresql-17-gosk`, `zfs rollback` to the snapshot, deploy the
previous nix generation, start the container.

A rollback also discards everything stored after the snapshot, including
minutes the receiver had already acknowledged to vessels - and an
acknowledged minute is one the vessel no longer offers. Rolling back
therefore has a follow-up: once the server is on the new version again, run
step 8 (seeding) for every vessel with `--from` set to the snapshot's date.
The digest comparison finds the acknowledged-then-lost minutes and puts them
back in the outbox. Do not take "outbox empty on the vessel" as proof the
server has the rows.

## 3. Broker ACLs

The outbox uses four new topic families. Vessels publish on
`outbox/data/<origin>` and `outbox/digest/<origin>` and subscribe to
`outbox/ack/<origin>` and `outbox/digest-reply/<origin>`, where `<origin>` is
`vessels.urn:mrn:imo:mmsi:<mmsi>`. The server does the opposite with a
wildcard.

### 3.1 The nix-managed broker (`the-production-server`, 10.64.0.1)

In `~/Code/nix`, on a new branch (`git switch -c transfer-outbox`):

`modules/nixos/services/sumo-mqtt.nix`, the per-client `acl` list (around line
126) becomes:

```nix
acl = [
  "write vessels/urn:mrn:imo:mmsi:${username}"
  "read request/vessels.urn:mrn:imo:mmsi:${username}"
  "write respond/vessels.urn:mrn:imo:mmsi:${username}"
  "write outbox/data/vessels.urn:mrn:imo:mmsi:${username}"
  "read outbox/ack/vessels.urn:mrn:imo:mmsi:${username}"
  "write outbox/digest/vessels.urn:mrn:imo:mmsi:${username}"
  "read outbox/digest-reply/vessels.urn:mrn:imo:mmsi:${username}"
];
```

`hosts/the-production-server/services/mosquitto/default.nix`, the `the-production-server`
user's `acl` gets:

```nix
"read outbox/data/#"
"write outbox/ack/#"
"read outbox/digest/#"
"write outbox/digest-reply/#"
```

and the `the-test-server` user, if it is to keep a live copy (step 5), gets
`"read outbox/data/#"`. The `munnik` and `smit` admin users can take
`"read outbox/#"` for debugging.

Mosquitto queues QoS 1 messages for a subscriber with a persistent session
that is offline, which is what the receiver now is while it restarts. The
default `max_queued_messages` of 1000 is enough for acknowledgements; for
deliveries queued while the receiver is down it is not, but an unacknowledged
delivery is simply sent again after `retry_after`, so raising it is optional:

```nix
services.mosquitto.settings = {
  max_queued_messages = 10000;
  persistence = true;
};
```

This deploys with step 4 (same host); do not deploy it separately unless you
want the ACLs to land first, which is also fine.

### 3.2 The legacy broker (10.24.0.1)

This broker is not in the nix repo; vessels on the old VPN reach
`broker.vpn.example.org` through it and `the-production-server`
reads it with the `10_24_0_1` user. Its ACL file has to get the same lines by
hand, for every vessel user that connects to it and for the `10_24_0_1` user:

```
user <mmsi>
topic write outbox/data/vessels.urn:mrn:imo:mmsi:<mmsi>
topic read outbox/ack/vessels.urn:mrn:imo:mmsi:<mmsi>
topic write outbox/digest/vessels.urn:mrn:imo:mmsi:<mmsi>
topic read outbox/digest-reply/vessels.urn:mrn:imo:mmsi:<mmsi>

user 10_24_0_1
topic read outbox/data/#
topic write outbox/ack/#
topic read outbox/digest/#
topic write outbox/digest-reply/#
```

Then reload mosquitto on that host (`kill -HUP $(pidof mosquitto)` on Alpine,
or `rc-service mosquitto reload`). Verify from `the-production-server` before going
on:

```sh
mosquitto_sub -h 10.24.0.1 -u 10_24_0_1 -P '<password>' -t 'outbox/data/#' -C 1 -W 5
```

An ACL denial shows up as a silent subscription that never receives anything,
so a timeout here without a vessel converted yet is expected; a `Connection
Refused: not authorised` is not.

## 4. the-production-server

### 4.1 Pin gosk

In `~/Code/nix/flake.nix` the input is `gosk = { url = "github:munnik/gosk"; }`.
Point it at the merged commit:

```sh
cd ~/Code/nix
nix flake lock --update-input gosk
git diff flake.lock   # check the gosk rev is the one from step 1
```

### 4.2 Services

`hosts/the-production-server/services/gosk/default.nix`: remove `transferRequest1`
and `transferRequest2` and add, in their place, one receiver and one digest
responder per broker. Everything else on the host (the two readers and
`writeDatabaseMapped`) stays as it is.

```nix
transferReceive1 = {
  args = "transfer receive";
  extraArgs = [ "--pmport" "127.0.0.1:9203" ];
  subscribeTo = [ ];
  config = {
    database = {
      url = "postgres://gosk:${config.sops.placeholder.postgresqlGoskPassword}@${postgresqlContainerAddress}:5432/gosk?sslmode=disable&application_name=gosk_transfer_receive";
      timeout = "1m";
    };
    mqtt = {
      username = "10_24_0_1";
      password = config.sops.placeholder.mosquittoPassword;
      url = "mqtt://10.24.0.1:1883";
      compress = true;
    };
  };
  after = [ "container@${postgresqlContainer}.service" ];
  partOf = [ "container@${postgresqlContainer}.service" ];
  requires = [ "container@${postgresqlContainer}.service" ];
};
transferReceive2 = {
  args = "transfer receive";
  extraArgs = [ "--pmport" "127.0.0.1:9204" ];
  subscribeTo = [ ];
  config = {
    database = {
      url = "postgres://gosk:${config.sops.placeholder.postgresqlGoskPassword}@${postgresqlContainerAddress}:5432/gosk?sslmode=disable&application_name=gosk_transfer_receive";
      timeout = "1m";
    };
    mqtt = {
      username = "the-production-server";
      password = config.sops.placeholder.mosquittoPassword;
      url = "mqtt://10.64.0.1:1883";
      compress = true;
    };
  };
  after = [ "container@${postgresqlContainer}.service" "mosquitto.service" ];
  partOf = [ "container@${postgresqlContainer}.service" ];
  requires = [ "container@${postgresqlContainer}.service" "mosquitto.service" ];
};
transferDigest1 = {
  args = "transfer digest";
  extraArgs = [ "--pmport" "127.0.0.1:9207" ];
  subscribeTo = [ ];
  config = {
    database = {
      url = "postgres://gosk:${config.sops.placeholder.postgresqlGoskPassword}@${postgresqlContainerAddress}:5432/gosk?sslmode=disable&application_name=gosk_transfer_digest";
      # One summary reads a vessel's whole range of history; a year of
      # one vessel is minutes, not seconds.
      timeout = "5m";
    };
    mqtt = {
      username = "10_24_0_1";
      password = config.sops.placeholder.mosquittoPassword;
      url = "mqtt://10.24.0.1:1883";
      compress = true;
    };
  };
  after = [ "container@${postgresqlContainer}.service" ];
  partOf = [ "container@${postgresqlContainer}.service" ];
  requires = [ "container@${postgresqlContainer}.service" ];
};
transferDigest2 = {
  args = "transfer digest";
  extraArgs = [ "--pmport" "127.0.0.1:9208" ];
  subscribeTo = [ ];
  config = {
    database = {
      url = "postgres://gosk:${config.sops.placeholder.postgresqlGoskPassword}@${postgresqlContainerAddress}:5432/gosk?sslmode=disable&application_name=gosk_transfer_digest";
      timeout = "5m";
    };
    mqtt = {
      username = "the-production-server";
      password = config.sops.placeholder.mosquittoPassword;
      url = "mqtt://10.64.0.1:1883";
      compress = true;
    };
  };
  after = [ "container@${postgresqlContainer}.service" "mosquitto.service" ];
  partOf = [ "container@${postgresqlContainer}.service" ];
  requires = [ "container@${postgresqlContainer}.service" "mosquitto.service" ];
};
```

Notes on what the module does with these:

- The module passes `--publishURL` to every processor whose args do not start
  with `write` (and are not `transfer request`). All four `transfer`
  subcommands accept it; `receive` publishes what it stores on it, the others
  ignore it. Nothing on this host needs to subscribe to the receivers: they
  write the database themselves. Do not add them to `writeDatabaseMapped`'s
  `subscribeTo`, or every row is written twice.
- `receive` acknowledges by default (`acknowledge = true`). Only one server
  may acknowledge a given broker's vessels, and that is this one.
- The `sleep_between_respond_deltas` default the module injects for
  `transfer respond` no longer matches anything and can be removed from
  `modules/nixos/services/gosk.nix` and `modules/tools/gosk.nix` in step 9.

### 4.3 Prometheus

`hosts/the-production-server/services/prometheus/default.nix`, the `gosk` job's
target list: replace the two `gosk_transfer_request*` entries with

```nix
{ port = 9203; instance = "gosk_transfer_receive1"; }
{ port = 9204; instance = "gosk_transfer_receive2"; }
{ port = 9207; instance = "gosk_transfer_digest1"; }
{ port = 9208; instance = "gosk_transfer_digest2"; }
```

The `gosk_transfer_node` job (vessels, port 9203 on the WireGuard address)
needs no change: the vessel's `transfer send` takes over that port in step 6.

The metrics the old protocol exported (`gosk_transfer_*`) are gone. The new
ones are:

| metric | where | meaning |
| --- | --- | --- |
| `gosk_outbox_depth{origin}` | vessel | minutes waiting to be acknowledged; the backlog |
| `gosk_outbox_deliveries_total`, `gosk_outbox_rows_sent_total` | vessel | what was sent |
| `gosk_outbox_acked_total`, `gosk_outbox_stale_acks_total`, `gosk_outbox_retried_total` | vessel | acknowledgements applied, ignored, and deliveries sent again |
| `gosk_outbox_received_total`, `gosk_outbox_stored_total`, `gosk_outbox_acks_sent_total`, `gosk_outbox_write_failures_total` | server | deliveries received, rows stored, acknowledgements sent, deliveries that could not be stored |
| `gosk_outbox_stale_deliveries_total` | server | deliveries not stored because a newer generation of their bucket already was (a late row on the vessel reset the bucket while the old delivery was still on its way) |
| `gosk_outbox_digest_*_total` | server | seeding activity |

`dashboards/vessel-health.py` has a `transfer_backlog` panel on the old
`gosk_transfer_*_queued` metrics; `gosk_outbox_depth` is its replacement, and
the "records transmitted" panel maps to `gosk_outbox_rows_sent_total`.

### 4.4 Deploy and verify

```sh
cd ~/Code/nix
check-hosts the-production-server
deploy --targets .#the-production-server
```

The deploy restarts every gosk unit; the first one to open the database runs
the three migrations. The units of the old version are stopped before the new
ones start (that is what a NixOS activation does for a changed unit), so no
old process writes a uuid column while they run - but check it: no
`gosk-*` unit may still be running the previous store path afterwards.

```sh
ssh the-production-server "systemctl show 'gosk-*' -p Id -p ActiveEnterTimestamp --no-pager | paste - -"
```

Every unit's `ActiveEnterTimestamp` must be after the deploy. Then:

```sh
ssh the-production-server
journalctl -u gosk-writeDatabaseMapped --since '10 min ago' | grep -i -E 'migrat|error' | head
systemctl status gosk-transferReceive1 gosk-transferReceive2 gosk-transferDigest1 gosk-transferDigest2 --no-pager | grep -E 'Active|gosk-'
```

Check the schema (the same `systemd-run` pattern works on `the-test-server`;
`machinectl shell` drops output):

```sh
sudo systemd-run -q -M postgresql-17-gosk --pipe --wait \
  --setenv=PATH=/run/current-system/sw/bin:/run/wrappers/bin \
  /bin/sh -c "psql -U postgres -d gosk -X -P pager=off -c \"SET search_path TO gosk, public; SELECT version, dirty FROM schema_migrations; SELECT column_name FROM information_schema.columns WHERE table_name = 'mapped_data_matching_context' ORDER BY ordinal_position; SELECT table_name FROM information_schema.tables WHERE table_name LIKE 'transfer%';\""
```

Expected: version `20261017120200`, `dirty = f`, no `uuid` or
`transfer_uuid` column, and `transfer_outbox` as the only `transfer%` table.
If `dirty` is `t`, a migration failed half way and every gosk unit is now
refusing to start (`Dirty database version N`). Do not clear the flag first.
In order:

1. Find the failing statement in the journal of whichever unit started
   first (`journalctl -u 'gosk-*' --since '10 min ago' | grep -B2 -A5 -i migrat`).
2. Stop all gosk units (`systemctl stop 'gosk-*'`) so nothing retries while
   you look.
3. Look at what the database actually holds: each migration runs in one
   transaction, so a failed one has normally rolled back completely and the
   schema is that of the previous version. Verify that with the queries
   above (the uuid columns and the `transfer_*` tables tell you which
   migration committed).
4. Only when the schema matches the previous version: fix the cause, then
   `UPDATE schema_migrations SET dirty = false, version = <previous>` and
   start the units again, which replays the failed migration.
5. If the schema matches neither version, roll back to the snapshot (step 2)
   rather than hand-repairing it.

The receivers log `MQTT connection established` with their client id
(`gosk-transferReceive-the-production-server-<username>`) and then wait. A vessel is
needed before they log anything else; that is step 6.

## 5. the-test-server

`the-test-server` holds a ZFS replica of the production database (nothing to
do: the schema change arrives with the next syncoid run) and, through its
`read mqtt` processor, ingests the live feed into its own test database. For
that second use, add a receiver that stores but never acknowledges:

```nix
transferReceive = {
  args = "transfer receive";
  subscribeTo = [ ];
  config = {
    database = { url = "<the test database's url>"; timeout = "1m"; };
    mqtt = {
      username = "the-test-server";
      password = config.sops.placeholder.mosquittoPassword;
      url = "mqtt://10.64.0.1:1883";
      compress = true;
    };
    acknowledge = false;
  };
};
```

`acknowledge = false` is the important line: an acknowledgement from this
host would let a vessel forget rows `the-production-server` never received. The
`the-test-server` broker user needs `read outbox/data/#` (step 3.1) and nothing
else. Deploy with `deploy --targets .#the-test-server`. Its own gosk processes
run the migrations on that test database when they restart.

If the test database is not wanted any more, skip this and remove
`the-test-server`'s `read vessels/#` grant in step 9 instead.

## 6. One vessel by hand

Pick a vessel that is reachable and whose data you can compare: a test node
such as `a-vessel` first, then a real one.

### 6.1 Changes per vessel

`hosts/<node>/services/gosk/gosk-transfer.nix`: `transferRespond` and
`writeMQTTTransferRespond` go; in their place:

```nix
services.gosk.processors = with gp; {
  transferSend = {
    args = "transfer send";
    subscribeTo = [ ];
    # The same port transferRespond published on, so the gosk_transfer_node
    # scrape job keeps finding this host.
    extraArgs = [ "--pmport" "${config.services.sumo-wireguard.address}:9203" ];
    config = {
      database = {
        url = "postgres://gosk:${config.sops.placeholder.goskPostgresqlGoskPassword}@${postgresqlContainerAddress}:5432/gosk?sslmode=disable&application_name=gosk_transfer_send";
        timeout = "1m";
      };
      mqtt = {
        username = config.vessel.mmsi;
        password = config.sops.placeholder.mosquittoPassword;
        url = "mqtt://broker.vpn.example.org:1883";
        compress = true;
      };
      origin = config.vessel.context;
      interval = "5s";
      max_buckets_per_cycle = 10;
      retry_after = "5m";
    };
    after = [ "container@${postgresqlContainer}.service" ];
    partOf = [ "container@${postgresqlContainer}.service" ];
    requires = [ "container@${postgresqlContainer}.service" ];
  };
};
```

`hosts/<node>/services/gosk/gosk-write.nix`:

- `writeDatabaseMapped.config` gets `outbox = true;`. Without it nothing is
  ever pending and the vessel goes silent in the cloud.
- `writeDatabaseMapped.config.batch_flush_interval` sets how long live data
  now takes to reach the cloud: a row is only visible to the sender once it
  is in the vessel's database. `"1m"` means dashboards lag up to a minute;
  `"10s"` keeps them close to what they are used to at the cost of six times
  as many (smaller) batches. `batch_flush_length = 10000` can stay.
- `writeMQTT` goes. The outbox is the transport now; keeping the old writer
  sends every row twice over the metered link.

`modules/tools/gosk.nix`'s `respondSleepBetweenDeltas` and the matching
branch in `modules/nixos/services/gosk.nix` are now dead and can go in step 9.

### 6.2 Deploy and verify

```sh
cd ~/Code/nix
check-hosts <node>
deploy --targets .#<node>
```

On the vessel:

```sh
ssh <node>
journalctl -u gosk-transferSend -f
```

Within a flush interval it logs `Sent an outbox delivery` with the bucket and
row count, and shortly after `An outbox delivery was acknowledged` with the
same bucket. On `the-production-server`, `journalctl -u gosk-transferReceive2 -f`
(or `1`, depending on the vessel's broker) shows `Stored and acknowledged an
outbox delivery` for that origin. Then:

```sh
curl -s http://<wg address>:9203/metrics | grep gosk_outbox_depth
```

should hover at one or two (the current minute and the one before it). A
depth that grows is either a receiver that is not acknowledging (its journal
says why) or an ACL that silently drops a topic (step 3).

On `the-production-server`, compare a minute of the vessel's data against the
vessel's own database with the `systemd-run` query from step 4.4:

```sql
SELECT count(*) FROM mapped_data
WHERE origin = 'vessels.urn:mrn:imo:mmsi:<mmsi>'
  AND time >= '<a minute that was acknowledged>' AND time < '<that minute> + 1 minute';
```

The two counts are equal for an acknowledged minute. If they are not, stop
here; the receiver's `WriteMappedNow` committed something other than what the
vessel sent, which should not be possible.

Let the vessel run for a day before step 7.

## 7. The rest of the fleet

Convert every remaining `hosts/node-*/services/gosk/gosk-transfer.nix` and
`gosk-write.nix` as in 6.1 (the files are near-identical across vessels, so a
`sed` over `hosts/node-*/services/gosk/` does most of it; check
`a-vessel` and `a-vessel`, whose write files carry their own
comments). Then:

```sh
cd ~/Code/nix
check-hosts            # every host, sequentially; or deploy-all's own check
git add -A && git commit -m 'Convert the fleet to the gosk outbox'
git push origin transfer-outbox
```

Open the pull request and merge it to `main`. The unattended run on
`the-office-server` deploys every node at 06:30 the next morning and retries the ones
that were unreachable hourly; `systemctl --failed` and `deploy-logs/` on
`the-office-server` show what is left. To go faster, `deploy-all -t node` from a
clean checkout of `main` does the same by hand.

A vessel that is offline on the morning of the deploy keeps running the old
version and keeps publishing on `vessels/<mmsi>`, which the old reader on
`the-production-server` still stores. Nothing is lost; it converts on the morning it
is back.

From the moment a vessel runs the new version its data arrives through the
outbox only. The old reader keeps running until the last vessel has
converted, and then goes in step 9.

## 8. Seeding: the data the cloud is already missing

The outbox covers what a vessel writes from the moment it is converted. The
gaps from before - the review on the `transfer-durability` branch measured
whole months absent for some vessels - are closed by seeding, once per vessel,
by hand on the vessel. It compares the two ends by summary and puts only the
differing minutes in the outbox; the sender then sends them, newest minute
first and the backlog behind it, ten minutes of backlog per five-second pass by
default (`max_buckets_per_cycle`).

A vessel only holds what its disk kept: `modules/nixos/containers/postgresql-gosk.nix`
drops the oldest `mapped_data` chunks when the disk passes 75 %, so what the
seeder finds is bounded by that. The summary query reads the vessel's whole
range per level; a year of a busy vessel is a scan of a hundred million rows
on each end, so seed a month or two at a time and let each run finish.

On the vessel, reuse the sender's configuration (it is the same file the
seeder needs):

```sh
ssh <node>
systemctl show -p LoadCredential -p ExecStart gosk-transferSend.service
```

That prints the rendered config file (`config.json:/run/secrets/rendered/...`)
and the gosk binary. Then, with both substituted:

```sh
sudo systemd-run --pipe --wait --collect --unit gosk-transferSeed \
  -p LoadCredential=config.json:/run/secrets/rendered/<file> \
  -p Slice=gosk-transferers.slice \
  /bin/sh -c 'exec /nix/store/<hash>-gosk-<version>/bin/gosk transfer seed --config "$CREDENTIALS_DIRECTORY/config.json" --from 2026-09-01 --to 2026-10-01'
```

`--from` and `--to` take a date or an RFC 3339 time; leaving `--from` out
starts at the oldest row the vessel holds and leaving `--to` out ends at the
current minute. The run logs one line per level (`Compared a level of
history` on the server, `Part of history differs` on the vessel) and finishes
with `Finished seeding the outbox from history`, with the number of minutes
it enqueued. It exits non-zero if any summary went unanswered three times
(`seed_timeout`, two minutes by default - raise it in the send config to
`"10m"` for a vessel on a slow link or a year-long range); run it again in
that case, it is idempotent.

Watch `gosk_outbox_depth` on the vessel fall afterwards. The sender's
`retry_after` and the receiver's one-transaction-per-delivery mean a seeded
backlog cannot be lost halfway, only delayed.

Vessels with nothing to recover cost one run that enqueues nothing. Do the
ones the review named first: `a-vessel` (April and June 2026
absent) and `a-vessel` (September and October 2025, July to
September 2026 short).

## 9. Cleanup, after the last vessel has converted

- `the-production-server`: remove `readMqtt1` and `readMqtt2` and the
  `subscribeTo` on `writeDatabaseMapped` that names them; remove the
  `read vessels/#`, `request/#` and `respond/#` ACL grants from
  `hosts/the-production-server/services/mosquitto/default.nix` and the per-client
  `request`/`respond` lines in `modules/nixos/services/sumo-mqtt.nix`; the
  same lines by hand on the legacy broker.
- `modules/nixos/services/gosk.nix`: the `transfer respond` special case in
  `processorConfig` and the `"transfer request" != p.args` clause of
  `isPublisher`. `modules/tools/gosk.nix`: `respondSleepBetweenDeltas`.
- `dashboards/vessel-health.py`: the `transfer_backlog` panel and the
  `gosk_transfer_*` targets, replaced as in 4.3; then `deploy-all-dashboards`.
- `~/Code/gosk`: the `transfer-durability` branch and its worktree carried
  the uuid-based design; its review document (`TRANSFER_REVIEW.md`) is still
  the record of why, but none of its code is needed now.
- Release the ZFS snapshot from step 2 once a week of outbox operation has
  passed: `sudo zfs destroy tank/data/postgresql-17-gosk@before-outbox-<date>`.
- The space the uuid columns took comes back as the columnstore policy
  recompresses chunks it touches and as chunks expire. To reclaim it on
  existing compressed chunks sooner, recompress them:
  `SELECT decompress_chunk(c); SELECT compress_chunk(c);` per chunk, oldest
  first, during a quiet hour - each one is a rewrite of that chunk and nothing
  else.

## What to watch in the first week

- `gosk_outbox_depth` per vessel: steady at one or two, or draining after a
  seed. Growing means acknowledgements are not arriving.
- `gosk_outbox_write_failures_total` on `the-production-server`: must stay zero. A
  non-zero value is a delivery the receiver could not store, and the journal
  has the cause; the vessel will keep re-sending it every `retry_after`.
- `gosk_outbox_stale_acks_total` on a vessel: a few per day is a sensor whose
  timestamps run behind and reset a bucket after it was sent. Many per
  minute is a sensor clock that is badly wrong and wants fixing, because every
  late row costs its whole minute again.
- `gosk_outbox_retried_total`: deliveries the broker or the receiver lost. Zero
  on a good link; a trickle on a bad one is what the retry is for.
