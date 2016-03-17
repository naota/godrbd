#!/bin/bash

echo /bin/true > /sys/module/drbd/parameters/usermode_helper
ip a add 127.0.0.1 dev lo
go test
sync
