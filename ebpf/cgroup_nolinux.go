//go:build !linux

package ebpf

func mountCgroup2() error {
	return nil
}
