package collector

type PodNetworkMetric struct {
	SrcIP        string `json:"src_ip"`
	SrcPod       string `json:"src_pod,omitempty"`
	SrcNamespace string `json:"src_namespace,omitempty"`
	SrcNode      string `json:"src_node,omitempty"`
	SrcZone      string `json:"src_zone,omitempty"`
	DstIP        string `json:"dst_ip"`
	DstIPType    string `json:"dst_ip_type"`
	DstPod       string `json:"dst_pod,omitempty"`
	DstNamespace string `json:"dst_namespace,omitempty"`
	DstNode      string `json:"dst_node,omitempty"`
	DstZone      string `json:"dst_zone,omitempty"`
	TxBytes      uint64 `json:"tx_bytes"`
	TxPackets    uint64 `json:"tx_packets"`
	RxBytes      uint64 `json:"rx_bytes"`
	RxPackets    uint64 `json:"rx_packets"`
	Proto        string `json:"proto"`
	TS           uint64 `json:"ts"`

	// lifetime is used to remove old pod metrics.
	// This will happen if there are no more conntrack entries
	// updating this metric.
	lifetime uint32
}
