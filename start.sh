#!/bin/env bash

make
pkill gosk

./gosk collect -p "tcp://127.0.0.1:6001" --config "config/collector/c32.yaml" &
./gosk collect -p "tcp://127.0.0.1:6002" --config "config/collector/zmg.yaml" &

sleep 1

./gosk map -p "tcp://127.0.0.1:6011" -s "tcp://127.0.0.1:6001" --config "config/mapper/c32.yaml" &
./gosk map -p "tcp://127.0.0.1:6012" -s "tcp://127.0.0.1:6002" --config "config/mapper/zmg.yaml" &> /dev/null &

sleep 1

./gosk proxy -p "tcp://127.0.0.1:6000" -s "tcp://127.0.0.1:6001" -s "tcp://127.0.0.1:6002" &
./gosk proxy -p "tcp://127.0.0.1:6010" -s "tcp://127.0.0.1:6011" -s "tcp://127.0.0.1:6012" &

sleep 1

./gosk write database raw -s "tcp://127.0.0.1:6000"  --config "config/writer/postgresql.yaml" &
./gosk write database mapped -s "tcp://127.0.0.1:6010"  --config "config/writer/postgresql.yaml" &

./gosk write mqtt -s "tcp://127.0.0.1:6010"  --config "config/writer/mqtt.yaml" &
./gosk read mqtt -p "tcp://127.0.0.1:6020"  --config "config/reader/mqtt.yaml" &

sleep 1

./gosk write signalk -s "tcp://127.0.0.1:6020"  --config "config/writer/signalk.yaml" &
