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
	foores := createMinor(t, "foo", 0, 0)

	err := foores.DeleteMinor()

	_, err = os.Stat("/dev/drbd0")
	if err == nil {
		foores.Down()
		t.Fatal("/dev/drbd0 not deleted")
	}

	foores.Down()
}

func createMinor(t *testing.T, name string, minor, vol int) *Resource {
	res, err := NewResource(name)
	if err != nil {
		t.Fatalf("Failed to create %s %s", name, err)
	}

	dev := fmt.Sprintf("/dev/drbd%d", minor)

	_, err = os.Stat(dev)
	if err == nil {
		res.Down()
		t.Fatal(dev, " exists")
	}

	err = res.CreateMinor(minor, vol)
	if err != nil {
		res.Down()
		t.Fatal("Failed to create minor", err)
	}

	_, err = os.Stat(dev)
	if err != nil {
		res.Down()
		t.Fatal(dev, " not created")
	}

	return res
}

func TestAttach(t *testing.T) {
	drbdDev := "/dev/drbd0"
	dataImg := "test-attach-data.img"
	metaImg := "test-attach-meta.img"
	size := int64(16) * 1024 * 1024 * 1024
	err := withResource(t, "foo", 0, 0, dataImg, size, metaImg, size,
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
	dataImgFoo := "test-data-foo.img"
	metaImgFoo := "test-meta-foo.img"
	size := int64(16) * 1024 * 1024 * 1024
	dataImgBar := "test-data-bar.img"
	metaImgBar := "test-meta-bar.img"

	err := withResource(t, "foo", 0, 0, dataImgFoo, size, metaImgFoo, size,
		func(foores *Resource, dataDevFoo, metaDevFoo string) error {
			return withResource(t, "bar", 1, 0, dataImgBar, size, metaImgBar, size,
				func(barres *Resource, dataDevBar, metaDevBar string) error {
					err := foores.SetPrimary(true)
					if err != nil {
						foores.Down()
						barres.Down()
						return err
					}

					fooport := "ipv4:127.0.0.1:7788"
					barport := "ipv4:127.0.0.1:7789"
					err = foores.Connect(fooport, barport)
					if err != nil {
						foores.Down()
						barres.Down()
						return err
					}
					err = barres.Connect(barport, fooport)
					if err != nil {
						foores.Down()
						barres.Down()
						return err
					}

					time.Sleep(1 * time.Second)

					err = foores.Disconnect(fooport, barport)
					if err != nil {
						foores.Down()
						barres.Down()
						return err
					}
					err = barres.Disconnect(barport, fooport)
					if err != nil {
						foores.Down()
						barres.Down()
						return err
					}

					return nil
				})
		})
	if err != nil {
		t.Fatal("TestConnect failed", err)
	}

}

func withResource(t *testing.T,
	name string, minor, vol int,
	dataImg string, dataSize int64, metaImg string, metaSize int64,
	f func(res *Resource, ddev, mdev string) error) error {

	res := createMinor(t, name, minor, vol)

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
