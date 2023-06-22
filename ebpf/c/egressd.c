#include <vmlinux.h>
#include <vmlinux_flavors.h>
#include <vmlinux_missing.h>

#include <bpf_helpers.h>

#include <maps.h>
#include <common.h>
#include <types.h>

char __license[] SEC("license") = "Dual MIT/GPL";

struct bpf_map_def SEC("maps") pkt_count = {
	.type        = BPF_MAP_TYPE_ARRAY,
	.key_size    = sizeof(u32),
	.value_size  = sizeof(u64),
	.max_entries = 1,
};

BPF_PERF_OUTPUT(events, 1024); // Events output map.

statfunc u64 sizeof_net_event_context_t(void)
{
    return sizeof(net_event_context_t) - sizeof(net_event_contextmd_t);
}

SEC("cgroup_skb/egress")
int count_egress_packets(struct __sk_buff *skb) {
	switch (skb->family) {
        case PF_INET:
        case PF_INET6:
            break;
        default:
            return 1;
    }
//    int family;
//    bpf_probe_read(&family, sizeof(family), &skb->family);
//	u32 key      = 0;
//	u64 init_val = 1;
//	u64 *count = bpf_map_lookup_elem(&pkt_count, &key);
//	if (!count) {
//		bpf_map_update_elem(&pkt_count, &key, &init_val, BPF_ANY);
//		return 1;
//	}
//	__sync_fetch_and_add(count, 1);

    net_event_context_t neteventctx = {};

    u32 size = skb->len;
    u64 flags = BPF_F_CURRENT_CPU;
    flags |= (u64) size << 32;

	bpf_perf_event_output(skb,
                          &events,
                          flags,
                          &neteventctx,
                          sizeof_net_event_context_t());

	return 1;
}
