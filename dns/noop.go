package dns

import (
	"context"

	"github.com/castai/egressd/pb"
)

type Noop struct{}

var _ DNSCollector = (*Noop)(nil)

func (t *Noop) Start(ctx context.Context) error {
	<-ctx.Done()
	return nil
}

func (d *Noop) Records() []*pb.IP2Domain {
	return nil
}
