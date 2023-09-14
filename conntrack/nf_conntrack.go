package conntrack

import (
	"fmt"
	"net"
	"os"
	"time"

	ct "github.com/florianl/go-conntrack"
	"github.com/sirupsen/logrus"
	"github.com/vishvananda/netns"
	"inet.af/netaddr"

	"github.com/castai/egressd/metrics"
)

type nfctDumper interface {
	Dump(t ct.Table, f ct.Family) ([]ct.Con, error)
	Close() error
}

var (
	accounting = "/hostproc/sys/net/netfilter/nf_conntrack_acct"
)

func initNetfilterAccounting() error {
	return os.WriteFile(accounting, []byte{'1'}, 0600)
}

func NewNetfilterClient(log logrus.FieldLogger) (Client, error) {
	err := initNetfilterAccounting()
	if err != nil {
		return nil, fmt.Errorf("initing nf_conntrack_acct: %w", err)
	}
	hostNs, err := netns.GetFromPid(1)
	if err != nil {
		return nil, err
	}
	nfct, err := ct.Open(&ct.Config{NetNS: int(hostNs)})
	if err != nil {
		return nil, fmt.Errorf("opening nfct: %w", err)
	}
	return &netfilterClient{
		log:        log,
		nfctDumper: nfct,
	}, nil
}

type netfilterClient struct {
	log        logrus.FieldLogger
	nfctDumper nfctDumper
}

func (n *netfilterClient) ListEntries(filter EntriesFilter) ([]*Entry, error) {
	sessions, err := n.nfctDumper.Dump(ct.Conntrack, ct.IPv4)
	if err != nil {
		return nil, fmt.Errorf("dumping nfct sessions: %w", err)
	}

	metrics.SetConntrackEntriesCount(float64(len(sessions)))

	now := time.Now().UTC()

	res := make([]*Entry, 0)
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

		entry := &Entry{
			Src:       netaddr.IPPortFrom(ipFromStdIP(*origin.Src), *origin.Proto.SrcPort),
			Dst:       netaddr.IPPortFrom(ipFromStdIP(*origin.Dst), *origin.Proto.DstPort),
			TxBytes:   *originCounter.Bytes,
			TxPackets: *originCounter.Packets,
			RxBytes:   *replyCounter.Bytes,
			RxPackets: *replyCounter.Packets,
			Proto:     *origin.Proto.Number,
		}
		if sess.Timeout != nil {
			entry.Lifetime = now.Add(time.Second * time.Duration(*sess.Timeout))
		}
		replySrc := netaddr.IPPortFrom(ipFromStdIP(*reply.Src), *reply.Proto.SrcPort)
		// Probably ClusterIP service, remap destination to actual pod IP.
		if entry.Dst != replySrc {
			entry.Dst = replySrc
		}
		if filter(entry) {
			res = append(res, entry)
		}

		// Cilium stores two records for single flow.
		entry2 := &Entry{
			Src:       entry.Dst,
			Dst:       entry.Src,
			TxBytes:   entry.RxBytes,
			TxPackets: entry.RxPackets,
			RxBytes:   entry.TxBytes,
			RxPackets: entry.TxPackets,
			Proto:     entry.Proto,
			Lifetime:  entry.Lifetime,
		}
		if filter(entry2) {
			res = append(res, entry2)
		}

	}
	if skippedEntriesCount > 0 {
		n.log.Debugf("skipped %d conntrack entries", skippedEntriesCount)
	}
	return res, nil
}

func (n *netfilterClient) Close() error {
	if n.nfctDumper != nil {
		return n.nfctDumper.Close()
	}
	return nil
}

func ipFromStdIP(ip net.IP) netaddr.IP {
	res, _ := netaddr.FromStdIP(ip)
	return res
}
