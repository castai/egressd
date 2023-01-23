package flusher

import (
	"hash/maphash"

	jsoniter "github.com/json-iterator/go"
	"github.com/samber/lo"

	"github.com/castai/egressd/collector"
)

type ConntrackDump struct {
	metrics map[uint64]*collector.PodNetworkMetric
	encoder *jsoniter.Encoder
}

func NewConntrackDump(encoder *jsoniter.Encoder) *ConntrackDump {
	return &ConntrackDump{
		metrics: map[uint64]*collector.PodNetworkMetric{},
		encoder: encoder,
	}
}

func (c *ConntrackDump) Dump(metricDelta collector.PodNetworkMetric) {
	id := entryKey(metricDelta)

	_, ok := c.metrics[id]
	if !ok {
		c.metrics[id] = &metricDelta
		return
	}
	c.metrics[id].TxBytes += metricDelta.TxBytes
	c.metrics[id].RxBytes += metricDelta.RxBytes
	c.metrics[id].TxPackets += metricDelta.TxPackets
	c.metrics[id].RxPackets += metricDelta.RxPackets
}

func (c *ConntrackDump) Flush() (int, error) {
	err := c.encoder.Encode(lo.Values(c.metrics))
	if err != nil {
		return 0, err
	}
	numMetrics := len(c.metrics)
	c.metrics = map[uint64]*collector.PodNetworkMetric{}
	return numMetrics, nil
}

var entryHash maphash.Hash

func entryKey(metric collector.PodNetworkMetric) uint64 {
	srcIP := []byte(metric.SrcIP)
	_, _ = entryHash.Write(srcIP[:])

	dstIP := []byte(metric.DstIP)
	_, _ = entryHash.Write(dstIP[:])

	_, _ = entryHash.Write([]byte(metric.Proto))
	res := entryHash.Sum64()

	entryHash.Reset()
	return res
}
