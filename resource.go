package drbd

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"text/scanner"
)

type Resource struct {
	name    string
	metadev string
	minor   int
	volume  int
}

func (r *Resource) Name() string {
	return r.name
}

func (r *Resource) Delete() error {
	err := r.DeleteMinor()
	if err != nil {
		return err
	}

	return exec.Command("drbdsetup", "del-resource", r.name).Run()
}

func (r *Resource) DeleteMinor() error {
	if r.minor < 0 {
		return nil
	}
	return exec.Command("drbdsetup", "del-minor", fmt.Sprintf("%d", r.minor)).Run()
}

func (r *Resource) CreateMinor(minor int, vol int) error {
	err := exec.Command("drbdsetup", "new-minor", r.name,
		fmt.Sprintf("%d", minor), fmt.Sprintf("%d", vol)).Run()
	if err != nil {
		return err
	}

	r.minor = minor
	r.volume = vol

	return nil
}

func (r *Resource) MinorDev() string {
	if r.minor < 0 {
		panic("minor not yet setuped")
	}
	return fmt.Sprintf("/dev/drbd%d", r.minor)
}

func (r *Resource) Scan() error {
	out, err := exec.Command("drbdsetup", "show", "all").CombinedOutput()
	if err != nil {
		return fmt.Errorf("drbdsetup show: %s\n%s", err, out)
	}

	var s scanner.Scanner
	s.Init(strings.NewReader(string(out)))
	var tok rune
	for tok != scanner.EOF {
		tok = s.Scan()
		switch s.TokenText() {
		case "volume", "minor":
			tok = s.Scan()
			if tok == scanner.EOF {
				return fmt.Errorf("drbdsetup show: scanning failure")
			}

			x, err := strconv.Atoi(s.TokenText())
			if err != nil {
				return err
			}

			if s.TokenText() == "volume" {
				r.volume = x
			} else {
				r.minor = x
			}
		}

	}

	return nil
}

func (r *Resource) Attach(datadev string, args ...string) error {
	err := ApplyActivityLog(r.metadev)
	if err != nil {
		return err
	}

	// FIXME: flexible
	args = append([]string{"attach", fmt.Sprintf("%d", r.minor),
		datadev, r.metadev, "flexible"}, args...)
	err = errorOut("drbdsetup attach", "drbdsetup", args...)
	if err != nil {
		return err
	}

	return nil
}

func (r *Resource) CreateMetaDev(metadev string) error {
	err := errorOut("drbdmeta create-md",
		"drbdmeta", "--force", r.MinorDev(),
		"v08", metadev, "flex-external", "create-md")
	if err != nil {
		return err
	}

	r.metadev = metadev
	return nil
}

func (r *Resource) Connect(local, remote string) error {
	return errorOut("drbdsetup connect",
		"drbdsetup", "connect", r.name, local, remote,
		"--protocol", "C", "--after-sb-0pri", "discard-zero-changes")
}

func (r *Resource) Disconnect(local, remote string) error {
	return errorOut("drbdsetup disconnect",
		"drbdsetup", "disconnect", local, remote)
}

func (r *Resource) Detach() error {
	return errorOut("drbdsetup detach", "drbdsetup", "detach", fmt.Sprintf("%d", r.minor))
}

func (r *Resource) Down() error {
	return errorOut("drbdsetup down", "drbdsetup", "down", r.name)
}

func (r *Resource) SetPrimary(force bool) error {
	args := []string{"primary", fmt.Sprintf("%d", r.minor)}
	if force {
		args = append(args, "--force")
	}

	return errorOut("drbdsetup primary", "drbdsetup", args...)
}

func (r *Resource) SetSecondary() error {
	return errorOut("drbdsetup secondary",
		"drbdsetup", "secondary", fmt.Sprintf("%d", r.minor))
}

func errorOut(prefix, cmd string, args ...string) error {
	out, err := exec.Command(cmd, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s\n%s", prefix, err, out)
	}
	return nil
}

// FIXME: skel function
func ApplyActivityLog(metadev string) error {
	return nil
}

func NewResource(name string) (*Resource, error) {
	r := new(Resource)
	r.name = name
	r.metadev = ""
	r.minor = -1
	r.volume = -1

	err := errorOut("drbdsetup new-resource",
		"drbdsetup", "new-resource", name)
	if err != nil {
		return nil, err
	}

	err = r.Scan()
	if err != nil {
		return nil, err
	}

	return r, nil
}

func ListResources() ([]string, error) {
	out, err := exec.Command("drbdsetup", "show", "all").CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("drbdsetup show: %s\n%s", err, out)
	}

	reses := []string{}

	var s scanner.Scanner
	s.Init(strings.NewReader(string(out)))
	var tok rune
	for tok != scanner.EOF {
		tok = s.Scan()
		if s.TokenText() == "resource" {
			tok = s.Scan()
			if tok == scanner.EOF {
				return nil, fmt.Errorf("drbdsetup show: scanning failure")
			}
			reses = append(reses, s.TokenText())
		}
	}

	return reses, nil
}
