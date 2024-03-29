//go:build linux

package ebpf

import (
	"fmt"
	"os"
	"syscall"
)

var mountPoint = "/cgroup2"

func init() {
	// When running with readonly root FS, we can't write to /*, so we
	// should be able to override the mount point.
	if p := os.Getenv("CGROUP2_MOUNT_PATH"); p != "" {
		mountPoint = p
	}
}

// mountCgroup2 mounts cgroup2.
// It should be used on systems that don't have cgroup2 mounted by default.
func mountCgroup2() error {
	err := os.Mkdir(mountPoint, 0755)
	if err != nil {
		return fmt.Errorf("creating directory at %q: %w", mountPoint, err)
	}
	// https://docs.kernel.org/admin-guide/cgroup-v2.html#mounting
	err = syscall.Mount("none", mountPoint, "cgroup2", 0, "")
	if err != nil {
		return fmt.Errorf("mounting cgroup2 at %q: %w", mountPoint, err)
	}
	return nil
}
