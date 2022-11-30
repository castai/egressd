package conntrack

import (
	"fmt"
	"net"

	ct "github.com/florianl/go-conntrack"
	"github.com/sirupsen/logrus"
	"inet.af/netaddr"
)

func NewNetfilterClient(log logrus.FieldLogger) (Client, error) {
	nfct, err := ct.Open(&ct.Config{})
	if err != nil {
		return nil, fmt.Errorf("opening nfct: %w", err)
	}
	return &netfilterClient{
		log:  log,
		nfct: nfct,
	}, nil
}

type netfilterClient struct {
	log  logrus.FieldLogger
	nfct *ct.Nfct
}

func (n *netfilterClient) ListEntries() (map[netaddr.IP][]Entry, error) {
	sessions, err := n.nfct.Dump(ct.Conntrack, ct.IPv4)
	if err != nil {
		return nil, fmt.Errorf("dumping nfct sessions: %w", err)
	}

	res := make(map[netaddr.IP][]Entry, 0)
	var skippedEntriesCount int
	for _, sess := range sessions {
		if sess.Origin == nil || sess.Origin.Src == nil || sess.Origin.Proto == nil || sess.Origin.Proto.SrcPort == nil || sess.Origin.Proto.DstPort == nil ||
			sess.Reply == nil || sess.Reply.Dst == nil || sess.Reply.Proto == nil || sess.Reply.Proto.SrcPort == nil || sess.Reply.Proto.DstPort == nil ||
			sess.CounterOrigin == nil || sess.CounterReply == nil {
			skippedEntriesCount++
			continue
		}

		origin := sess.Origin
		originCounter := sess.CounterOrigin
		reply := sess.Reply
		replyCounter := sess.CounterReply
		in := Entry{
			Src:       netaddr.IPPortFrom(ipFromStdIP(*origin.Src), *origin.Proto.SrcPort),
			Dst:       netaddr.IPPortFrom(ipFromStdIP(*origin.Dst), *origin.Proto.DstPort),
			TxBytes:   *originCounter.Bytes,
			TxPackets: *originCounter.Packets,
			RxBytes:   *replyCounter.Bytes,
			RxPackets: *replyCounter.Packets,
			Proto:     *origin.Proto.Number,
			Reply:     false,
		}
		res[in.Src.IP()] = append(res[in.Src.IP()], in)

		out := Entry{
			Src:       netaddr.IPPortFrom(ipFromStdIP(*reply.Src), *reply.Proto.SrcPort),
			Dst:       netaddr.IPPortFrom(ipFromStdIP(*reply.Dst), *reply.Proto.DstPort),
			TxBytes:   *replyCounter.Bytes,
			TxPackets: *replyCounter.Packets,
			RxBytes:   *originCounter.Bytes,
			RxPackets: *originCounter.Packets,
			Proto:     *reply.Proto.Number,
			Reply:     true,
		}
		res[out.Src.IP()] = append(res[out.Src.IP()], out)
	}
	if skippedEntriesCount > 0 {
		n.log.Warnf("skipped %d conntrack entries", skippedEntriesCount)
	}
	return res, nil
}

func (n *netfilterClient) Close() error {
	if n.nfct != nil {
		return n.nfct.Close()
	}
	return nil
}

func ipFromStdIP(ip net.IP) netaddr.IP {
	res, _ := netaddr.FromStdIP(ip)
	return res
}
