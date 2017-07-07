package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"syscall"
)

var (
	name         = flag.String("name", "", "User name")
	destroy      = flag.Bool("destroy", false, "Run in destroy mode?")
	scriptsQuota = flag.Int("scripts_quota", 1*1024*1024, "Scripts quota")
	privateQuota = flag.Int("private_quota", 20*1024*1024, "Private quota")
)

var zfs string = "/sbin/zfs"

var rootStorageVolume string = "kobun4-executor-storage"

var nameRegexp = regexp.MustCompile(`^[a-z0-9_-]{1,20}$`)

func newCommand(name string, arg ...string) *exec.Cmd {
	cmd := exec.Command(name, arg...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd
}

func newCommandNoStdout(name string, arg ...string) *exec.Cmd {
	cmd := exec.Command(name, arg...)
	cmd.Stderr = os.Stderr
	return cmd
}

func setuid(uid int) (err error) {
	_, _, e1 := syscall.RawSyscall(syscall.SYS_SETUID, uintptr(uid), 0, 0)
	if e1 != 0 {
		err = e1
	}
	return
}

func destroyVolume(storageVolume string) error {
	return newCommand(zfs, "destroy", "-r", storageVolume).Run()
}

func main() {
	runtime.GOMAXPROCS(1)
	runtime.LockOSThread()

	flag.Parse()

	uid := syscall.Getuid()
	gid := syscall.Getgid()

	if !nameRegexp.MatchString(*name) {
		panic("name is invalid")
	}

	storageVolume := filepath.Join(rootStorageVolume, *name)

	if err := setuid(0); err != nil {
		panic(err)
	}

	if *destroy {
		if err := destroyVolume(storageVolume); err != nil {
			panic(err)
		}
		return
	}

	if err := newCommand(zfs, "create", storageVolume).Run(); err != nil {
		panic(err)
	}

	var outErr error
	defer func() {
		if outErr != nil {
			destroyVolume(storageVolume)
			panic(outErr)
		}
	}()

	stdout, err := newCommandNoStdout(zfs, "get", "-H", "-o", "value", "mountpoint", storageVolume).Output()
	if err != nil {
		outErr = err
		return
	}

	storageRoot := strings.TrimSuffix(string(stdout), "\n")

	if err := os.Chown(storageRoot, uid, gid); err != nil {
		outErr = err
		return
	}

	if err := newCommand(zfs, "create", "-o", fmt.Sprintf("quota=%d", *scriptsQuota), filepath.Join(storageVolume, "scripts")).Run(); err != nil {
		outErr = err
		return
	}

	if err := os.Chown(filepath.Join(storageRoot, "scripts"), uid, gid); err != nil {
		outErr = err
		return
	}

	if err := newCommand(zfs, "create", "-o", fmt.Sprintf("quota=%d", *privateQuota), filepath.Join(storageVolume, "private")).Run(); err != nil {
		outErr = err
		return
	}

	if err := os.Chown(filepath.Join(storageRoot, "private"), uid, gid); err != nil {
		outErr = err
		return
	}
}
