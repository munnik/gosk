#!/bin/bash

docker network rm gosk
docker network create -d bridge gosk

docker run \
-p 5432:5432 \
-d \
-e "POSTGRES_PASSWORD=topsecret" \
--name timescaledb \
--network gosk \
timescale/timescaledb:latest-pg12

docker run \
-p 5051:5051 \
-d \
-e "PGADMIN_DEFAULT_EMAIL=martijndemunnik@protonmail.com" \
-e "PGADMIN_DEFAULT_PASSWORD=topsecret" \
-e "PGADMIN_LISTEN_PORT=5051" \
--rm \
--name pgadmin \
--network gosk \
dpage/pgadmin4

docker run \
-p 3000:3000 \
-d \
--name grafana \
--network gosk \
-e "GF_INSTALL_PLUGINS=grafana-worldmap-panel" \
grafana/grafana

docker ps -a
docker network inspect gosk