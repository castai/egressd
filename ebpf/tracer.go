package ebpf

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/sirupsen/logrus"
)

func NewTracer(log logrus.FieldLogger) *Tracer {
	return &Tracer{log: log}
}

type Tracer struct {
	log logrus.FieldLogger
}

func (t *Tracer) Run(ctx context.Context) error {
	// Allow the current process to lock memory for eBPF resources.
	if err := rlimit.RemoveMemlock(); err != nil {
		return err
	}

	// Load pre-compiled programs and maps into the kernel.
	objs := bpfObjects{}
	var customBTF *btf.Spec
	if _, err := os.Stat("/sys/kernel/btf/vmlinux"); errors.Is(err, os.ErrNotExist) {
		t.log.Warnf("btf file not found at /sys/kernel/btf/vmlinux, will load custom which may not work correctly")
		// TODO: Load btpf specific to actual kernel version.
		//btf, err := os.Open("/app/btfs/5.4.0-96-generic.btf")
		spec, err := btf.LoadSpec("/app/cmd/tracer/5.8.0-63-generic.btf")
		if err != nil {
			return err
		}
		customBTF = spec
	}

	if err := loadBpfObjects(&objs, &ebpf.CollectionOptions{
		Maps: ebpf.MapOptions{},
		Programs: ebpf.ProgramOptions{
			KernelTypes: customBTF,
		},
		MapReplacements: nil,
	}); err != nil {
		return fmt.Errorf("loading objects: %v", err)
	}
	defer objs.Close()

	// Get the first-mounted cgroupv2 path.
	cgroupPath, err := detectCgroupPath()
	if err != nil {
		return err
	}

	// Link the count_egress_packets program to the cgroup.
	l, err := link.AttachCgroup(link.CgroupOptions{
		Path:    cgroupPath,
		Attach:  ebpf.AttachCGroupInetEgress,
		Program: objs.CountEgressPackets,
	})
	if err != nil {
		return err
	}
	defer l.Close()

	t.log.Println("Counting packets...")

	// Read loop reporting the total amount of times the kernel
	// function was entered, once per second.
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	reader, err := perf.NewReader(objs.Events, 1024)
	if err != nil {
		return err
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		record, err := reader.Read()
		if err != nil {
			return err
		}
		fmt.Println(record.RawSample)
	}
	//
	//for {
	//	select {
	//	case <-ctx.Done():
	//		return ctx.Err()
	//	case <-ticker.C:
	//		var value uint64
	//		if err := objs.PktCount.Lookup(uint32(0), &value); err != nil {
	//			return err
	//		}
	//		log.Printf("number of packets: %d\n", value)
	//	}
	//}
}

func detectCgroupPath() (string, error) {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		// example fields: cgroup2 /sys/fs/cgroup/unified cgroup2 rw,nosuid,nodev,noexec,relatime 0 0
		fields := strings.Split(scanner.Text(), " ")
		if len(fields) >= 3 && fields[2] == "cgroup2" {
			return fields[1], nil
		}
	}

	return "", errors.New("cgroup2 not mounted")
}
