package collector

type PodNetworkMetric struct {
	SrcIP        string `json:"src_ip"`
	SrcPod       string `json:"src_pod"`
	SrcNamespace string `json:"src_namespace"`
	SrcNode      string `json:"src_node"`
	SrcZone      string `json:"src_zone"`
	DstIP        string `json:"dst_ip"`
	DstPod       string `json:"dst_pod"`
	DstNamespace string `json:"dst_namespace"`
	DstNode      string `json:"dst_node"`
	DstZone      string `json:"dst_zone"`
	TxBytes      uint64 `json:"tx_bytes"`
	TxPackets    uint64 `json:"tx_packets"`
	Proto        string `json:"proto"`
	Ts           uint64 `json:"ts"`
}
