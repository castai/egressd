package conntrack

import (
	"bufio"
	"net"
	"strconv"
	"strings"
	"testing"

	ct "github.com/florianl/go-conntrack"
	"github.com/stretchr/testify/require"
	"inet.af/netaddr"
)

func TestNetfilterConntrack(t *testing.T) {
	r := require.New(t)
	dumper := &mockNfctDumper{}
	client := &netfilterClient{
		nfctDumper: dumper,
	}

	res, err := client.ListEntries(All())
	r.NoError(err)
	r.Len(res, 86)

	byIP := filterEntriesBySrcIP(res, netaddr.MustParseIP("10.20.18.11"))
	r.Len(byIP, 1)
	r.Equal(Entry{
		Src:       netaddr.MustParseIPPort("10.20.18.11:33886"),
		Dst:       netaddr.MustParseIPPort("10.20.3.34:8080"),
		TxBytes:   79679,
		TxPackets: 1531,
		RxBytes:   79609,
		RxPackets: 1530,
		Proto:     6,
	}, byIP[0])

	byIPViaService := filterEntriesBySrcIP(res, netaddr.MustParseIP("10.20.3.38"))
	r.Len(byIPViaService, 1)
	r.Equal(Entry{
		Src:       netaddr.MustParseIPPort("10.20.3.38:56930"),
		Dst:       netaddr.MustParseIPPort("10.20.19.12:14268"),
		TxBytes:   30863406,
		TxPackets: 16339,
		RxBytes:   1261341,
		RxPackets: 13606,
		Proto:     6,
	}, byIPViaService[0])

	resByIpFilter, err := client.ListEntries(FilterBySrcIP(map[netaddr.IP]struct{}{
		netaddr.MustParseIP("10.20.3.38"): {},
	}))
	r.NoError(err)
	r.Len(resByIpFilter, 1)
}

func filterEntriesBySrcIP(entries []*Entry, ip netaddr.IP) []Entry {
	res := make([]Entry, 0)
	for _, e := range entries {
		if e.Src.IP() == ip {
			res = append(res, *e)
		}
	}
	return res
}

type mockNfctDumper struct {
}

func (m mockNfctDumper) Dump(t ct.Table, f ct.Family) ([]ct.Con, error) {
	raw := `ipv4     2 tcp      6 86387 ESTABLISHED src=10.20.3.34 dst=10.20.3.5 sport=40234 dport=8080 packets=48392 bytes=2661548 src=10.20.3.5 dst=10.20.3.34 sport=8080 dport=40234 packets=59243 bytes=1830866063 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86387 ESTABLISHED src=10.20.3.34 dst=10.30.0.1 sport=45542 dport=443 packets=857 bytes=51781 src=172.16.0.2 dst=10.20.3.34 sport=443 dport=45542 packets=853 bytes=56772 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86398 ESTABLISHED src=10.20.3.34 dst=10.20.18.16 sport=46848 dport=8080 packets=32504 bytes=2769233 src=10.20.18.16 dst=10.20.3.34 sport=8080 dport=46848 packets=20254 bytes=3028962 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86394 ESTABLISHED src=10.20.9.26 dst=10.20.3.34 sport=58102 dport=8090 packets=266 bytes=109154 src=10.20.3.34 dst=10.20.9.26 sport=8090 dport=58102 packets=169 bytes=24445 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86397 ESTABLISHED src=10.20.3.34 dst=10.30.0.1 sport=45578 dport=443 packets=855 bytes=50271 src=172.16.0.2 dst=10.20.3.34 sport=443 dport=45578 packets=852 bytes=57757 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86390 ESTABLISHED src=10.20.3.34 dst=10.30.0.1 sport=45564 dport=443 packets=858 bytes=51833 src=172.16.0.2 dst=10.20.3.34 sport=443 dport=45564 packets=854 bytes=56824 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86391 ESTABLISHED src=10.20.3.34 dst=10.20.3.33 sport=56738 dport=8080 packets=14343 bytes=1137896 src=10.20.3.33 dst=10.20.3.34 sport=8080 dport=56738 packets=7241 bytes=1113318 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 2 CLOSE src=10.20.3.34 dst=10.3.0.23 sport=40474 dport=5432 packets=30 bytes=3037 src=10.3.0.23 dst=10.20.3.34 sport=5432 dport=40474 packets=22 bytes=3602 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86395 ESTABLISHED src=10.20.3.34 dst=10.3.0.23 sport=40478 dport=5432 packets=1170 bytes=137092 src=10.3.0.23 dst=10.20.3.34 sport=5432 dport=40478 packets=1558 bytes=22577809 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 30 TIME_WAIT src=10.20.9.26 dst=10.20.3.34 sport=48330 dport=8090 packets=17 bytes=4346 src=10.20.3.34 dst=10.20.9.26 sport=8090 dport=48330 packets=15 bytes=1302 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 103 TIME_WAIT src=10.20.9.26 dst=10.20.3.34 sport=50530 dport=8090 packets=9 bytes=1113 src=10.20.3.34 dst=10.20.9.26 sport=8090 dport=50530 packets=7 bytes=457 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 1 CLOSE src=10.20.3.34 dst=10.3.0.23 sport=44842 dport=5432 packets=29 bytes=2983 src=10.3.0.23 dst=10.20.3.34 sport=5432 dport=44842 packets=23 bytes=3654 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86395 ESTABLISHED src=10.20.3.34 dst=10.20.5.150 sport=34370 dport=8080 packets=33215 bytes=2821119 src=10.20.5.150 dst=10.20.3.34 sport=8080 dport=34370 packets=19163 bytes=2994120 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86386 ESTABLISHED src=10.20.3.34 dst=10.20.18.9 sport=59162 dport=8080 packets=1529 bytes=79575 src=10.20.18.9 dst=10.20.3.34 sport=8080 dport=59162 packets=1528 bytes=79505 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86390 ESTABLISHED src=10.20.8.4 dst=10.20.3.34 sport=37562 dport=2112 packets=2425 bytes=313101 src=10.20.3.34 dst=10.20.8.4 sport=2112 dport=37562 packets=1572 bytes=5557262 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86392 ESTABLISHED src=10.20.3.34 dst=172.251.111.11 sport=51000 dport=443 packets=2044 bytes=252076 src=172.251.111.11 dst=10.10.0.59 sport=443 dport=51000 packets=1902 bytes=261348 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86395 ESTABLISHED src=10.20.19.59 dst=10.20.3.34 sport=44642 dport=8090 packets=110 bytes=46655 src=10.20.3.34 dst=10.20.19.59 sport=8090 dport=44642 packets=70 bytes=9723 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86392 ESTABLISHED src=10.20.3.34 dst=142.250.111.111 sport=38900 dport=443 packets=16997 bytes=4125022 src=142.250.111.111 dst=10.10.0.59 sport=443 dport=38900 packets=22922 bytes=11474116 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86392 ESTABLISHED src=10.20.3.34 dst=142.250.111.111 sport=38916 dport=443 packets=5697 bytes=522828 src=142.250.111.111 dst=10.10.0.59 sport=443 dport=38916 packets=12531 bytes=12563582 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86393 ESTABLISHED src=10.20.3.34 dst=10.20.3.33 sport=56750 dport=8080 packets=31143 bytes=2546454 src=10.20.3.33 dst=10.20.3.34 sport=8080 dport=56750 packets=16477 bytes=4332077 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86392 ESTABLISHED src=10.20.3.34 dst=10.3.0.23 sport=53984 dport=5432 packets=333 bytes=111616 src=10.3.0.23 dst=10.20.3.34 sport=5432 dport=53984 packets=265 bytes=325962 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86385 ESTABLISHED src=10.20.5.128 dst=10.20.3.34 sport=47484 dport=8080 packets=1547 bytes=80511 src=10.20.3.34 dst=10.20.5.128 sport=8080 dport=47484 packets=1546 bytes=80441 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86394 ESTABLISHED src=10.20.13.103 dst=10.20.3.34 sport=50300 dport=8090 packets=40 bytes=11541 src=10.20.3.34 dst=10.20.13.103 sport=8090 dport=50300 packets=36 bytes=487561 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86391 ESTABLISHED src=10.20.13.103 dst=10.20.3.34 sport=35328 dport=8090 packets=53 bytes=26446 src=10.20.3.34 dst=10.20.13.103 sport=8090 dport=35328 packets=37 bytes=4844 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86393 ESTABLISHED src=10.20.3.34 dst=10.30.0.1 sport=45552 dport=443 packets=859 bytes=51830 src=172.16.0.2 dst=10.20.3.34 sport=443 dport=45552 packets=856 bytes=53495 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86389 ESTABLISHED src=10.20.3.34 dst=10.30.0.1 sport=45556 dport=443 packets=858 bytes=51758 src=172.16.0.2 dst=10.20.3.34 sport=443 dport=45556 packets=855 bytes=59627 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86392 ESTABLISHED src=10.20.3.34 dst=10.20.18.12 sport=37414 dport=8080 packets=13994 bytes=1113535 src=10.20.18.12 dst=10.20.3.34 sport=8080 dport=37414 packets=7129 bytes=1109202 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86391 ESTABLISHED src=10.20.13.103 dst=10.20.3.34 sport=60978 dport=8090 packets=85 bytes=34065 src=10.20.3.34 dst=10.20.13.103 sport=8090 dport=60978 packets=68 bytes=492124 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86398 ESTABLISHED src=10.20.9.26 dst=10.20.3.34 sport=60144 dport=8090 packets=41 bytes=24451 src=10.20.3.34 dst=10.20.9.26 sport=8090 dport=60144 packets=35 bytes=3600 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86388 ESTABLISHED src=10.20.9.26 dst=10.20.3.34 sport=40972 dport=8090 packets=22 bytes=13004 src=10.20.3.34 dst=10.20.9.26 sport=8090 dport=40972 packets=20 bytes=1763 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86386 ESTABLISHED src=10.20.19.59 dst=10.20.3.34 sport=33456 dport=8090 packets=151 bytes=57400 src=10.20.3.34 dst=10.20.19.59 sport=8090 dport=33456 packets=105 bytes=12586 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86399 ESTABLISHED src=10.20.3.34 dst=10.30.0.215 sport=56930 dport=14268 packets=16339 bytes=30863406 src=10.20.19.12 dst=10.20.3.34 sport=14268 dport=56930 packets=13606 bytes=1261341 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86388 ESTABLISHED src=10.20.18.11 dst=10.20.3.34 sport=33886 dport=8080 packets=1531 bytes=79679 src=10.20.3.34 dst=10.20.18.11 sport=8080 dport=33886 packets=1530 bytes=79609 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86395 ESTABLISHED src=10.20.9.26 dst=10.20.3.34 sport=50514 dport=8090 packets=22 bytes=7733 src=10.20.3.34 dst=10.20.9.26 sport=8090 dport=50514 packets=16 bytes=2914 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86392 ESTABLISHED src=10.20.3.34 dst=172.251.111.11 sport=52454 dport=443 packets=2048 bytes=257773 src=172.251.111.11 dst=10.10.0.59 sport=443 dport=52454 packets=1914 bytes=239419 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86387 ESTABLISHED src=10.20.13.103 dst=10.20.3.34 sport=44220 dport=8090 packets=751 bytes=389026 src=10.20.3.34 dst=10.20.13.103 sport=8090 dport=44220 packets=527 bytes=1035119 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86387 ESTABLISHED src=10.20.3.34 dst=10.20.5.134 sport=43766 dport=8080 packets=32150 bytes=1835133 src=10.20.5.134 dst=10.20.3.34 sport=8080 dport=43766 packets=56826 bytes=1474690804 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86387 ESTABLISHED src=10.20.19.59 dst=10.20.3.34 sport=34658 dport=8090 packets=41 bytes=10021 src=10.20.3.34 dst=10.20.19.59 sport=8090 dport=34658 packets=37 bytes=487337 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86386 ESTABLISHED src=10.20.3.34 dst=10.30.0.1 sport=45538 dport=443 packets=856 bytes=51744 src=172.16.0.2 dst=10.20.3.34 sport=443 dport=45538 packets=853 bytes=56378 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 45 TIME_WAIT src=10.20.13.103 dst=10.20.3.34 sport=36700 dport=8090 packets=9 bytes=1279 src=10.20.3.34 dst=10.20.13.103 sport=8090 dport=36700 packets=7 bytes=520 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86399 ESTABLISHED src=10.20.3.34 dst=10.20.18.6 sport=55092 dport=8080 packets=1651 bytes=97051 src=10.20.18.6 dst=10.20.3.34 sport=8080 dport=55092 packets=1589 bytes=85736 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86399 ESTABLISHED src=10.20.3.34 dst=10.20.3.8 sport=48526 dport=8080 packets=1675 bytes=99181 src=10.20.3.8 dst=10.20.3.34 sport=8080 dport=48526 packets=1606 bytes=86741 [ASSURED] mark=0 zone=0 use=2
ipv4     2 tcp      6 86399 ESTABLISHED src=10.20.3.38 dst=10.30.0.215 sport=56930 dport=14268 packets=16339 bytes=30863406 src=10.20.19.12 dst=10.20.3.38 sport=14268 dport=56930 packets=13606 bytes=1261341 [ASSURED] mark=0 zone=0 use=2
`

	scanner := bufio.NewScanner(strings.NewReader(raw))
	var res []ct.Con
	for scanner.Scan() {
		line := scanner.Text()
		parsedLine := parseConntrackLine(line)
		res = append(res, parsedLine)
	}
	return res, nil
}

func (m mockNfctDumper) Close() error {
	return nil
}

type conntrackEntry struct {
	proto     uint8
	reqSrc    netaddr.IPPort
	reqDst    netaddr.IPPort
	respSrc   netaddr.IPPort
	respDst   netaddr.IPPort
	txBytes   uint64
	txPackets uint64
	rxBytes   uint64
	rxPackets uint64
}

func parseConntrackLine(line string) ct.Con {
	fields := strings.Fields(line)
	_ = fields[0]                       // Network layer protocol (eg. ipv4)
	_ = fields[1]                       // Network layer protocol number (eg. 2)
	_ = fields[2]                       // Transmission layer name (eg. tcp)
	proto, _ := strconv.Atoi(fields[3]) // Transmission layer number (eg. 6)
	_ = fields[4]                       // Seconds until entry is invalidated.
	maybeConnState := fields[5]         // Connection state. Optional.
	fieldsStart := 6
	if maybeConnState[0] == 's' && maybeConnState[1] == 'r' && maybeConnState[2] == 'c' {
		// There is no state field. Start fields from 5 pos.
		fieldsStart = 5
	}

	entry := conntrackEntry{
		proto: uint8(proto),
	}

	var srcIP, dstIP, srcPort, dstPort, packets string
	var reqAccDone bool
	for _, field := range fields[fieldsStart:] {
		index := strings.IndexByte(field, '=')
		if index == -1 {
			continue
		}
		key := field[:index]
		val := field[index+1:]
		switch key {
		case "src":
			srcIP = val
		case "dst":
			dstIP = val
		case "sport":
			srcPort = val
		case "dport":
			dstPort = val
			srcAddr := parseIPPort(srcIP, srcPort)
			dstAddr := parseIPPort(dstIP, dstPort)
			if entry.reqSrc.IsZero() {
				entry.reqSrc = srcAddr
				entry.reqDst = dstAddr
			} else {
				entry.respSrc = srcAddr
				entry.respDst = dstAddr
			}
		case "packets":
			packets = val
		case "bytes":
			packetsNum, _ := strconv.Atoi(packets)
			bytesNum, _ := strconv.Atoi(val)
			if !reqAccDone {
				reqAccDone = true
				entry.txPackets = uint64(packetsNum)
				entry.txBytes = uint64(bytesNum)
			} else {
				entry.rxPackets = uint64(packetsNum)
				entry.rxBytes = uint64(bytesNum)
			}
		}
	}

	return ct.Con{
		Origin: &ct.IPTuple{
			Src: toStdIP(entry.reqSrc),
			Dst: toStdIP(entry.reqDst),
			Proto: &ct.ProtoTuple{
				Number:  &entry.proto,
				SrcPort: toPortPtr(entry.reqSrc.Port()),
				DstPort: toPortPtr(entry.reqDst.Port()),
			},
		},
		Reply: &ct.IPTuple{
			Src: toStdIP(entry.respSrc),
			Dst: toStdIP(entry.respDst),
			Proto: &ct.ProtoTuple{
				Number:  &entry.proto,
				SrcPort: toPortPtr(entry.respSrc.Port()),
				DstPort: toPortPtr(entry.respDst.Port()),
			},
		},
		CounterOrigin: &ct.Counter{
			Packets: toCounterValPtr(entry.txPackets),
			Bytes:   toCounterValPtr(entry.txBytes),
		},
		CounterReply: &ct.Counter{
			Packets: toCounterValPtr(entry.rxPackets),
			Bytes:   toCounterValPtr(entry.rxBytes),
		},
	}
}

func parseIPPort(ip, port string) netaddr.IPPort {
	portint, _ := strconv.Atoi(port)
	return netaddr.IPPortFrom(netaddr.MustParseIP(ip), uint16(portint))
}

func toStdIP(addr netaddr.IPPort) *net.IP {
	stdIP := addr.IP().IPAddr().IP
	return &stdIP
}

func toPortPtr(port uint16) *uint16 {
	return &port
}

func toCounterValPtr(v uint64) *uint64 {
	return &v
}
