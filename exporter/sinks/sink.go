package sinks

import (
	"context"

	"github.com/castai/egressd/pb"
)

type Sink interface {
	Push(ctx context.Context, batch *pb.PodNetworkMetricBatch) error
}
