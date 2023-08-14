package ebpf

import "github.com/sirupsen/logrus"

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
