package ebpf

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/perf"
	"github.com/cilium/ebpf/rlimit"
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/sirupsen/logrus"
)

type Config struct {
	QueueSize         int
	CustomBTFFilePath string
}

func NewTracer(log logrus.FieldLogger, cfg Config) *Tracer {
	if cfg.QueueSize == 0 {
		cfg.QueueSize = 1000
	}
	return &Tracer{
		log:    log.WithField("component", "ebpf_tracer"),
		cfg:    cfg,
		events: make(chan DNSEvent, cfg.QueueSize),
	}
}

type Tracer struct {
	log    logrus.FieldLogger
	cfg    Config
	events chan DNSEvent
}

func (t *Tracer) Run(ctx context.Context) error {
	// Allow the current process to lock memory for eBPF resources.
	if err := rlimit.RemoveMemlock(); err != nil {
		return err
	}

	t.log.Debug("running")
	defer t.log.Debug("stopping")

	objs := bpfObjects{}
	var customBTF *btf.Spec
	if t.cfg.CustomBTFFilePath != "" {
		t.log.Debugf("loading custom btf from path %q", t.cfg.CustomBTFFilePath)
		spec, err := btf.LoadSpec(t.cfg.CustomBTFFilePath)
		if err != nil {
			return err
		}
		customBTF = spec
	}

	// Load pre-compiled programs and maps into the kernel.
	if err := loadBpfObjects(&objs, &ebpf.CollectionOptions{
		Maps: ebpf.MapOptions{},
		Programs: ebpf.ProgramOptions{
			KernelTypes: customBTF,
		},
		MapReplacements: nil,
	}); err != nil {
		return fmt.Errorf("loading objects: %w", err)
	}
	defer objs.Close()

	// Get the first-mounted cgroupv2 path.
	cgroupPath, err := detectCgroupPath()
	if err != nil {
		return err
	}

	l, err := link.AttachCgroup(link.CgroupOptions{
		Path:    cgroupPath,
		Attach:  ebpf.AttachCGroupInetIngress,
		Program: objs.CgroupIngress,
	})
	if err != nil {
		return err
	}
	defer l.Close()

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

		if len(record.RawSample) < 4 {
			t.log.Warnf("skipping too small event: %d bytes", len(record.RawSample))
			continue
		}

		// First 4 bytes now reserved for payload size. See net_event_context in types.h for full structure.
		event, err := parseEvent(record.RawSample[4:])
		if err != nil {
			t.log.Errorf("parsing event: %v", err)
			continue
		}

		select {
		case t.events <- event:
		default:
			t.log.Warn("dropping event, queue is full")
			continue
		}
	}
}

func (t *Tracer) Events() <-chan DNSEvent {
	return t.events
}

func IsKernelBTFAvailable() bool {
	_, err := os.Stat("/sys/kernel/btf/vmlinux")
	return err == nil
}

func parseEvent(data []byte) (DNSEvent, error) {
	packet := gopacket.NewPacket(
		data,
		layers.LayerTypeIPv4,
		gopacket.Default,
	)

	var res DNSEvent
	if packet == nil {
		return res, errors.New("parsing packet")
	}

	appLayer := packet.ApplicationLayer()
	if appLayer == nil {
		return res, errors.New("layer L7 is missing")
	}

	dns, ok := appLayer.(*layers.DNS)
	if !ok {
		return res, fmt.Errorf("expected dns layer, actual type %T", appLayer)
	}

	return DNSEvent{
		Questions: dns.Questions,
		Answers:   dns.Answers,
	}, nil
}

func detectCgroupPath() (string, error) {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return "", err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), " ")
		if len(fields) >= 3 && fields[2] == "cgroup2" {
			return fields[1], nil
		}
	}

	return "", errors.New("cgroup2 not mounted")
}
