# Install
## Install this package

```
go get github.com/naota/godrbd
```

## Install Kernel Module

(TBD)

## Install Userland Programs

Please install drbd utility programs.

```
$ sudo apt-get install drbd8-utils
```

## Setup Kernel Parameter

A parameter of `drbd` kernel module, `usermode_helper` should be set
to `/bin/true` as following to prevent the default usermode helper
complain about missing configuration file.

```
# modprobe drbd usermode_helper=/bin/true
or
# echo /bin/true > /sys/module/drbd/parameters/usermode_helper
```

# How to Use


```go
package Main

import (
	"github.com/naota/godrbd"
	"log"
)

func main() {
	resourceName := "resouce"
	minor := 0
	volume := 0

	// Creating a resource and /dev/drbd0

	res, err := drbd.NewResource(resourceName)
	if err != nil {
		log.Fatal(err)
	}

	err = res.CreateMinor(minor, volume)
	if err != nil {
		// Down() will remove all configuration information
		// from the device.
		res.Down()
		log.Fatal(err)
	}

	// Here, you can access /dev/drbd0

	// Attaching /dev/drbd0 to underling devices

	dataDev := "/dev/vg/drbd-data"
	metaDev := "/dev/vg/drbd-meta"

	// Setup metadata device on metaDev
	// * This will erase data on the device *
	err = res.CreateMetaDev(metaDev)
	if err != nil {
		res.Down()
		log.Fatal(err)
	}

	// Attach() can take optional arguments for `drbdsetup attach`
	err = res.Attach(dataDev, "--on-io-error=detach")
	if err != nil {
		res.Down()
		log.Fatal(err)
	}

	// The argument specify forcing set primary or not.
	err = res.SetPrimary(true)
	if err != nil {
		res.Down()
		log.Fatal(err)
	}

	// Here, writes to /dev/drbd0 will direct to /dev/vg/drbd-data

	// Connecting the resource to a peer

	myport := "192.168.0.1"             // default to ipv4 and port 7788
	peerport := "ipv4:192.168.0.2:7789" // or specify manually
	err = res.Connect(myport, peerport)
	if err != nil {
		res.Down()
		log.Fatal(err)
	}

	// Here, the connection should be established and the
	// synchronization process should be working

	// Clean up in a sane manner
	err = res.SetSecondary()
	if err != nil {
		log.Fatal(err)
	}

	err = res.Detach()
	if err != nil {
		log.Fatal(err)
	}
}
```
