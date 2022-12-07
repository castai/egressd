package conntrack

import (
	"bufio"
	"io"
	"strconv"
	"strings"

	"github.com/castai/egressd/metrics"
	"github.com/sirupsen/logrus"
	"inet.af/netaddr"
)

func NewNetfilterClient(log logrus.FieldLogger, conntrackOpener func() (io.ReadCloser, error)) Client {
	return &netfilterClient{
		log:             log,
		conntrackOpener: conntrackOpener,
	}
}

type netfilterClient struct {
	log             logrus.FieldLogger
	conntrackOpener func() (io.ReadCloser, error)
}

func (n *netfilterClient) ListEntries(filter EntriesFilter) (map[netaddr.IP][]Entry, error) {
	file, err := n.conntrackOpener()
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	res := make(map[netaddr.IP][]Entry, 0)
	var count int
	for scanner.Scan() {
		// Split the line into fields
		entry := parseConntrackLine(scanner.Text())
		count++
		if entry.txBytes == 0 {
			continue
		}

		egress := Entry{
			Src:       entry.reqSrc,
			Dst:       entry.reqDst,
			TxBytes:   entry.txBytes,
			TxPackets: entry.txPackets,
			RxBytes:   entry.rxBytes,
			RxPackets: entry.rxPackets,
			Proto:     entry.proto,
			Ingress:   false,
		}
		if filter(&egress) {
			res[egress.Src.IP()] = append(res[egress.Src.IP()], egress)
		}
		ingress := Entry{
			Src:       entry.respSrc,
			Dst:       entry.respDst,
			TxBytes:   entry.txBytes,
			TxPackets: entry.txPackets,
			RxBytes:   entry.rxBytes,
			RxPackets: entry.rxPackets,
			Proto:     entry.proto,
			Ingress:   true,
		}
		if filter(&ingress) {
			res[ingress.Src.IP()] = append(res[ingress.Src.IP()], ingress)
		}

		// ClusterIP nat. Add as egress, but use destination as ingress source.
		if egress.Dst != ingress.Src {
			egress2 := Entry{
				Src:       egress.Src,
				Dst:       ingress.Src,
				TxBytes:   egress.TxBytes,
				TxPackets: egress.TxPackets,
				RxBytes:   egress.RxBytes,
				RxPackets: egress.RxPackets,
				Proto:     egress.Proto,
				Ingress:   egress.Ingress,
			}
			if filter(&egress2) {
				res[egress2.Src.IP()] = append(res[egress2.Src.IP()], egress2)
			}
		}
	}

	metrics.SetConntrackEntriesCount(float64(count))
	return res, nil
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

func parseConntrackLine(line string) conntrackEntry {
	fields := strings.Fields(line)
	_ = fields[0]                       // Network layer protocol (eg. ipv4)
	_ = fields[1]                       // Network layer protocol number (eg. 2)
	_ = fields[2]                       // Transmission layer name (eg. tcp)
	proto, _ := strconv.Atoi(fields[3]) // Transmission layer number (eg. 6)
	_ = fields[4]                       // Seconds until entry is invalidated.
	_ = fields[5]                       // Connection state.

	entry := conntrackEntry{
		proto: uint8(proto),
	}

	var srcIP, dstIP, srcPort, dstPort, packets string
	var reqAccDone bool
	for _, field := range fields[6:] {
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

	return entry
}

func parseIPPort(ip, port string) netaddr.IPPort {
	portint, _ := strconv.Atoi(port)
	return netaddr.IPPortFrom(netaddr.MustParseIP(ip), uint16(portint))
}
