#!/bin/env bash

#make
#pkill gosk

./gosk collect -u "tcp://127.0.0.1:6001" --config "config/NMEA0183/wheelhouse.json" &
./gosk collect -u "tcp://127.0.0.1:6002" --config "config/MODBUS/c32.json" &

sleep 5

./gosk map -u "tcp://127.0.0.1:6004" -s "tcp://127.0.0.1:6001" --config "config/NMEA0183/wheelhouse.json" &
./gosk map -u "tcp://127.0.0.1:6005" -s "tcp://127.0.0.1:6002" --config "config/MODBUS/c32.json" &

sleep 5

./gosk proxy -u "tcp://127.0.0.1:6003" -s "tcp://127.0.0.1:6001" -s "tcp://127.0.0.1:6002" &
./gosk proxy -u "tcp://127.0.0.1:6006" -s "tcp://127.0.0.1:6004" -s "tcp://127.0.0.1:6005" &

sleep 5

./gosk database raw -s "tcp://127.0.0.1:6003" &
./gosk database keyvalue -s "tcp://127.0.0.1:6006" &

# ./gosk ws -s "tcp://127.0.0.1:6006" &