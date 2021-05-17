#!/bin/env bash

pkill gosk

make

./gosk collect -u "tcp://127.0.0.1:6001" --config "config/NMEA0183/wheelhouse.json" &
sleep 1
./gosk collect -u "tcp://127.0.0.1:6002" --config "config/MODBUS/c32.json" &
sleep 1

./gosk map -u "tcp://127.0.0.1:6004" -s "tcp://127.0.0.1:6001" --config "config/NMEA0183/wheelhouse.json" &
sleep 1
./gosk map -u "tcp://127.0.0.1:6005" -s "tcp://127.0.0.1:6002" --config "config/MODBUS/c32.json" &
sleep 1

./gosk proxy -u "tcp://127.0.0.1:6003" -s "tcp://127.0.0.1:6001" -s "tcp://127.0.0.1:6002" &
sleep 1
./gosk proxy -u "tcp://127.0.0.1:6006" -s "tcp://127.0.0.1:6004" -s "tcp://127.0.0.1:6005" &
sleep 1

./gosk database raw -s "tcp://127.0.0.1:6003" &
sleep 1
./gosk database keyvalue -s "tcp://127.0.0.1:6006" &
sleep 1

./gosk ws -s "tcp://127.0.0.1:6006" &