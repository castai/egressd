//go:build linux

package ebpf

import (
	"fmt"
	"os"
	"syscall"
)

const mountPoint = "/mnt/cgroup2"

func mountCgroup2() error {
	err := os.Mkdir(mountPoint, 0755)
	if err != nil {
		return fmt.Errorf("creating directory at %q: %w", mountPoint, err)
	}
	err = syscall.Mount("none", mountPoint, "cgroup2", 0, "")
	if err != nil {
		return fmt.Errorf("mounting cgroup2 at %q: %w", mountPoint, err)
	}
	return nil
}
