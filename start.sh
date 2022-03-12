#!/bin/env bash

#make
#pkill gosk

./gosk collect -u "tcp://127.0.0.1:6001" --config "config/collector/nmea0183/zmg.json" &
./gosk collect -u "tcp://127.0.0.1:6002" --config "config/collector/modbus/ampero.json" &

sleep 5

./gosk map -u "tcp://127.0.0.1:6011" -s "tcp://127.0.0.1:6001" --config "config/NMEA0183/wheelhouse.json" &
./gosk map -u "tcp://127.0.0.1:6012" -s "tcp://127.0.0.1:6002" --config "config/MODBUS/c32.json" &

sleep 5

./gosk proxy -u "tcp://127.0.0.1:6000" -s "tcp://127.0.0.1:6001" -s "tcp://127.0.0.1:6002" &
./gosk proxy -u "tcp://127.0.0.1:6010" -s "tcp://127.0.0.1:6011" -s "tcp://127.0.0.1:6012" &

sleep 5

./gosk database raw -s "tcp://127.0.0.1:6000" &
./gosk database keyvalue -s "tcp://127.0.0.1:6010" &

./gosk http -s "tcp://127.0.0.1:6010"  --config "config/writer/http.yaml" &