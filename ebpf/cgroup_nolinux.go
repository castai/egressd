//go:build !linux

package ebpf

// nolint:unused
func mountCgroup2() error {
	return nil
}
