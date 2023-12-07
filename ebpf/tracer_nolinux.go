//go:build !linux

package ebpf

import (
	"context"
)

func (t *Tracer) Run(ctx context.Context) error {
	panic("not implemented on non-linux")
}

func (t *Tracer) Events() <-chan DNSEvent {
	return t.events
}

func IsKernelBTFAvailable() bool {
	return false
}

func InitCgroupv2() error {
	return nil
}
