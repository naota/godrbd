package drbd

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestResource(t *testing.T) {
	foores, err := NewResource("foo")
	if err != nil {
		t.Fatal("cannot create resource foo", err)
	}
	if foores.Name() != "foo" {
		foores.Down()
		t.Fatal("foo: name does not match")
	}

	barres, err := NewResource("bar")
	if err != nil {
		foores.Down()
		barres.Down()
		t.Fatal("cannout create resource bar", err)
	}
	if barres.Name() != "bar" {
		foores.Down()
		barres.Down()
		t.Fatal("bar: name does not match")
	}

	reses, err := ListResources()
	if err != nil {
		foores.Down()
		barres.Down()
		t.Fatalf("cannot list resources: %s", err)
	}
	if len(reses) != 2 || reses[0] != "foo" || reses[1] != "bar" {
		foores.Down()
		barres.Down()
		t.Fatal("resource list not match")
	}

	err = foores.Delete()
	if err != nil {
		foores.Down()
		barres.Down()
		t.Fatal("failed to delete foo")
	}
	err = barres.Delete()
	if err != nil {
		foores.Down()
		barres.Down()
		t.Fatal("failed to delete bar")
	}

	reses, err = ListResources()
	if err != nil {
		t.Fatal("cannot list resources")
	}
	if len(reses) != 0 {
		t.Fatal("resource list should be empty")
	}
}

func TestMinor(t *testing.T) {
	foores, err := createMinor("foo", 0, 0)
	if err != nil {
		t.Fatal("TestMinor: ", err)
	}

	err = foores.DeleteMinor()

	_, err = os.Stat("/dev/drbd0")
	if err == nil {
		foores.Down()
		t.Fatal("/dev/drbd0 not deleted")
	}

	foores.Down()
}

func createMinor(name string, minor, vol int) (*Resource, error) {
	res, err := NewResource(name)
	if err != nil {
		return nil, fmt.Errorf("Failed to create %s: %s", name, err)
	}

	dev := fmt.Sprintf("/dev/drbd%d", minor)

	_, err = os.Stat(dev)
	if err == nil {
		res.Down()
		return nil, fmt.Errorf("%s exists", dev)
	}

	err = res.CreateMinor(minor, vol)
	if err != nil {
		res.Down()
		return nil, fmt.Errorf("Failed to create minor: %s", err)
	}

	_, err = os.Stat(dev)
	if err != nil {
		res.Down()
		return nil, fmt.Errorf("%s not created", dev)
	}

	return res, nil
}

func TestAttach(t *testing.T) {
	drbdDev := "/dev/drbd0"
	dataImg := "test-attach-data.img"
	metaImg := "test-attach-meta.img"
	size := int64(16) * 1024 * 1024 * 1024
	err := withResource("foo", 0, 0, dataImg, size, metaImg, size,
		func(foores *Resource, dataDev, metaDev string) error {
			err := foores.SetPrimary(true)
			if err != nil {
				foores.Down()
				return err
			}

			dlen := 1024
			data := make([]byte, dlen)
			_, err = rand.Read(data)
			if err != nil {
				foores.Down()
				return err
			}

			file, err := os.OpenFile(drbdDev, os.O_WRONLY, 0600)
			if err != nil {
				foores.Down()
				return err
			}

			_, err = file.Write(data)
			if err != nil {
				foores.Down()
				return err
			}
			file.Sync()
			file.Close()

			// cmp result
			file2, err := os.Open(dataImg)
			if err != nil {
				foores.Down()
				return err
			}

			data2 := make([]byte, dlen)
			_, err = file2.Read(data2)
			if err != nil {
				file2.Close()
				foores.Down()
				return err
			}
			file2.Close()

			for i := 0; i < dlen; i++ {
				if data[i] != data2[i] {
					foores.Down()
					return errors.New(fmt.Sprintf(
						"data mismatch at %d %d <-> %d",
						i, data[i], data2[i]))
				}
			}

			err = foores.SetSecondary()
			if err != nil {
				foores.Down()
				return err
			}

			err = foores.Detach()
			if err != nil {
				foores.Down()
				return err
			}

			return nil
		})
	if err != nil {
		t.Fatal("TestAttach failed", err)
	}

}

func withLoop(image string, size int64, f func(dev string) error) error {
	// check existence
	_, err := os.Stat(image)
	if err == nil {
		return errors.New(fmt.Sprintf("%s exists", image))
	}

	// create image
	_, err = os.Create(image)
	if err != nil {
		return err
	}

	err = os.Truncate(image, size)
	if err != nil {
		return err
	}

realloc:
	out, err := exec.Command("losetup", "-f").CombinedOutput()
	if err != nil {
		os.Remove(image)
		return errors.New(string(out))
	}
	device := string(out[0 : len(out)-1])

	// setup loop dev
	out, err = exec.Command("losetup", device, image).CombinedOutput()
	if err != nil {
		busystr := "failed to set up loop device: Device or resource busy\n"
		if string(out)[len(out)-len(busystr):len(out)] == busystr {
			// retry
			goto realloc
		}
		os.Remove(image)
		return errors.New(string(out))
	}

	err = f(device)

	// tear down loop dev and image
	// FIXME: error handling
	exec.Command("losetup", "-d", device).Run()
	os.Remove(image)
	return err
}

func TestConnect(t *testing.T) {
	size := int64(16) * 1024 * 1024 * 1024

	confFoo := new(ResourceConfig)
	confFoo.name = "foo"
	confFoo.minor = 0
	confFoo.volume = 0
	confFoo.dataImg = "test-data-foo.img"
	confFoo.dataSize = size
	confFoo.metaImg = "test-meta-foo.img"
	confFoo.metaSize = size
	confFoo.port = "ipv4:127.0.0.1:7788"

	confBar := new(ResourceConfig)
	confBar.name = "bar"
	confBar.minor = 1
	confBar.volume = 0
	confBar.dataImg = "test-data-bar.img"
	confBar.dataSize = size
	confBar.metaImg = "test-meta-bar.img"
	confBar.metaSize = size
	confBar.port = "ipv4:127.0.0.1:7789"

	err := withConnection(confFoo, confBar,
		func(foores, barres *Resource) error {
			time.Sleep(1 * time.Second)

			err := foores.Disconnect(confFoo.port, confBar.port)
			if err != nil {
				foores.Down()
				barres.Down()
				return err
			}
			err = barres.Disconnect(confBar.port, confFoo.port)
			if err != nil {
				foores.Down()
				barres.Down()
				return err
			}

			return nil
		})
	if err != nil {
		t.Fatal("TestConnect failed", err)
	}

}

func withResource(name string, minor, vol int,
	dataImg string, dataSize int64, metaImg string, metaSize int64,
	f func(res *Resource, ddev, mdev string) error) error {

	res, err := createMinor(name, minor, vol)
	if err != nil {
		return err
	}

	return withLoop(dataImg, dataSize,
		func(dataDev string) error {
			return withLoop(metaImg, metaSize,
				func(metaDev string) error {
					err := res.CreateMetaDev(metaDev)
					if err != nil {
						return err
					}

					err = res.Attach(dataDev, "--on-io-error=detach")
					if err != nil {
						res.Down()
						return err
					}

					err = f(res, dataDev, metaDev)
					if err != nil {
						res.Down()
						return err
					}

					res.Down()
					return nil
				})
		})
}

type ResourceConfig struct {
	name     string
	minor    int
	volume   int
	dataImg  string
	dataSize int64
	metaImg  string
	metaSize int64
	port     string
}

func withConnection(confX, confY *ResourceConfig,
	f func(resX, resY *Resource) error) error {
	return withResource(confX.name, confX.minor, confX.volume,
		confX.dataImg, confX.dataSize, confX.metaImg, confX.metaSize,
		func(resX *Resource, dataDevX, metaDevX string) error {
			return withResource(confY.name, confY.minor, confY.volume,
				confY.dataImg, confY.dataSize, confY.metaImg, confY.metaSize,
				func(resY *Resource, dataDevY, metaDevY string) error {
					err := resX.SetPrimary(true)
					if err != nil {
						resX.Down()
						resY.Down()
						return err
					}

					err = resX.Connect(confX.port, confY.port)
					if err != nil {
						resX.Down()
						resY.Down()
						return err
					}
					err = resY.Connect(confY.port, confX.port)
					if err != nil {
						resX.Down()
						resY.Down()
						return err
					}

					return f(resX, resY)
				})
		})
}
