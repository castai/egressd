package collector

import (
	"context"

	"github.com/sirupsen/logrus"
)

type DNSStore interface {
	Run(ctx context.Context) error
}

func NewEBPFDNSStore(ctx context.Context, log logrus.FieldLogger) (DNSStore, error) {
	return &ebpfDNSStore{
		log: log,
	}, nil
}

type ebpfDNSStore struct {
	log logrus.FieldLogger
}

func (s *ebpfDNSStore) Run(ctx context.Context) error {
	return nil
}
