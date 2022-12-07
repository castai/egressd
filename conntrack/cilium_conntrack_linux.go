//go:build linux

package conntrack

import (
	"fmt"
	"os"

	"github.com/castai/egressd/metrics"
	"github.com/cilium/cilium/pkg/bpf"
	"github.com/cilium/cilium/pkg/maps/ctmap"
	"inet.af/netaddr"
)

func listRecords(maps []interface{}, filter EntriesFilter) (map[netaddr.IP][]Entry, error) {
	entries := make(map[netaddr.IP][]Entry)

	var fetchedCount int
	for _, m := range maps {
		m := m.(ctmap.CtMap)
		path, err := m.Path()
		if err == nil {
			err = m.Open()
		}
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("unable to open map %s: %w", path, err)
			}
		}
		defer m.Close()
		cb := func(key bpf.MapKey, v bpf.MapValue) {
			fetchedCount++
			k := key.(ctmap.CtKey).ToHost().(*ctmap.CtKey4Global)
			if k.NextHeader == 0 {
				return
			}

			srcIP := k.DestAddr.IP() // Addresses are swapped due to cilium issue #21346.
			dstIP := k.SourceAddr.IP()
			val := v.(*ctmap.CtEntry)
			record := Entry{
				Src:       netaddr.IPPortFrom(netaddr.IPv4(srcIP[0], srcIP[1], srcIP[2], srcIP[3]), k.SourcePort),
				Dst:       netaddr.IPPortFrom(netaddr.IPv4(dstIP[0], dstIP[1], dstIP[2], dstIP[3]), k.DestPort),
				TxBytes:   val.TxBytes,
				TxPackets: val.TxPackets,
				RxBytes:   val.RxBytes,
				RxPackets: val.RxPackets,
				Proto:     uint8(k.NextHeader),
				Ingress:   k.Flags&ctmap.TUPLE_F_IN != 0,
			}
			if filter(&record) {
				entries[record.Src.IP()] = append(entries[record.Src.IP()], record)
			}
		}
		if err = m.DumpWithCallback(cb); err != nil {
			return nil, fmt.Errorf("error while collecting BPF map entries: %w", err)
		}
	}
	metrics.SetConntrackEntriesCount(float64(fetchedCount))
	return entries, nil
}

func initMaps() []interface{} {
	ctmap.InitMapInfo(2<<18, 2<<17, true, false, true)
	maps := ctmap.GlobalMaps(true, false)
	ctMaps := make([]interface{}, len(maps))
	for i, m := range maps {
		ctMaps[i] = m
	}
	return ctMaps
}
