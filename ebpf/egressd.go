//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc $BPF_CLANG -cflags $BPF_CFLAGS bpf ./egressd.c -- -nostdinc -I./headers

package ebpf

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/rlimit"
)

func NewEgressd() (*Egressd, error) {
	// Allow the current process to lock memory for eBPF resources.
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, err
	}
	var customBTF *btf.Spec
	if _, err := os.Stat("/sys/kernel/btf/vmlinux"); errors.Is(err, os.ErrNotExist) {
		// TODO: Load btpf specific to actual kernel version.
		//btf, err := os.Open("/app/btfs/5.4.0-96-generic.btf")
		spec, err := btf.LoadSpec("/app/generic.btf")
		if err != nil {
			return nil, fmt.Errorf("loading btf spec: %w", err)
		}
		customBTF = spec
	}
	opts := &ebpf.CollectionOptions{
		Maps: ebpf.MapOptions{
			PinPath:        "",
			LoadPinOptions: ebpf.LoadPinOptions{},
		},
		Programs: ebpf.ProgramOptions{
			LogLevel:    0,
			LogSize:     0,
			KernelTypes: customBTF,
		},
	}
	var objects bpfObjects
	if err := loadBpfObjects(&objects, opts); err != nil {
		return nil, fmt.Errorf("loading objects: %w", err)
	}

	return &Egressd{objects: &objects}, nil
}

type Egressd struct {
	objects *bpfObjects
}

func (e *Egressd) Start(ctx context.Context) error {
	dnsReader, err := perf.NewReader(e.objects.DnsEvents, 4096)
	if err != nil {
		return fmt.Errorf("opening dns reader: %w", err)
	}
	defer dnsReader.Close()

	udpRecvLink, err := link.Kprobe("udp_recvmsg", e.objects.UdpRecvmsg, &link.KprobeOptions{})
	if err != nil {
		return err
	}
	defer udpRecvLink.Close()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		r, err := dnsReader.Read()
		if errors.Is(err, perf.ErrClosed) {
			fmt.Println("ErrClosed")
			return nil
		}
		fmt.Println("dns sample", r.RawSample)
	}
}
