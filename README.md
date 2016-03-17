# Setup Kernel Parameter

A parameter of `drbd` kernel module, `usermode_helper` should be set
to `/bin/true` as following to prevent the default usermode helper
complain about missing configuration file.

```
# modprobe drbd usermode_helper=/bin/true
or
# echo /bin/true > /sys/module/drbd/parameters/usermode_helper
```
