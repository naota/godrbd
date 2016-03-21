#!/bin/bash

echo /bin/true > /sys/module/drbd/parameters/usermode_helper
ip a add 127.0.0.1 dev lo
dmesg -C
go test
res=$?
sync
dmesg
exit $res
