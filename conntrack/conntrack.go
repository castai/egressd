package conntrack

import (
	"strconv"

	"inet.af/netaddr"
)

type Entry struct {
	Src       netaddr.IPPort
	Dst       netaddr.IPPort
	TxBytes   uint64
	TxPackets uint64
	RxBytes   uint64
	RxPackets uint64
	Proto     uint8
	Reply     bool
}

type EntriesFilter func(e Entry) bool

func All() EntriesFilter {
	return func(e Entry) bool {
		return true
	}
}

type Client interface {
	ListEntries() (map[netaddr.IP][]Entry, error)
	Close() error
}

var protoNames = map[uint8]string{
	0:  "ANY",
	1:  "ICMP",
	6:  "TCP",
	17: "UDP",
	58: "ICMPv6",
}

func ProtoString(p uint8) string {
	if _, ok := protoNames[p]; ok {
		return protoNames[p]
	}
	return strconv.Itoa(int(p))
}
