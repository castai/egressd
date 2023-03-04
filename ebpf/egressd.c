// +build ignore

// Note: This file is licenced differently from the rest of the project
// SPDX-License-Identifier: GPL-2.0
// Copyright (C) Egressd authors.

#include "vmlinux.h"
#include "bpf_endian.h"
#include "bpf_helpers.h"
#include "bpf_tracing.h"
#include "custom_helpers.h"

char __license[] SEC("license") = "Dual MIT/GPL";

#define MAC_HEADER_SIZE 14;
#define AF_INET    2
#define BPF_F_INDEX_MASK 0xffffffffULL
#define BPF_F_CURRENT_CPU BPF_F_INDEX_MASK
#define BPF_F_NO_PREALLOC	 (1U << 0)

// Flags for BPF_MAP_UPDATE_ELEM command.
#define BPF_ANY		0 // create new element or update existing
#define BPF_NOEXIST	1 // create new element if it didn't exist
#define BPF_EXIST	2 // update existing element

struct {
	__uint(type, BPF_MAP_TYPE_PERF_EVENT_ARRAY);
	__uint(key_size, sizeof(u32));
	__uint(value_size, sizeof(u32));
} dns_events SEC(".maps");

#define DNS_PACKET_MAP_KEY 0
#define MAX_PKT 512
struct dns_packet_t {
    u8  pkt[MAX_PKT];
};

struct {
	__uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
	__uint(key_size, sizeof(u32));
	__uint(value_size, sizeof(struct dns_packet_t));
	__uint(max_entries, 1);
} dns_packet_map SEC(".maps");

static __always_inline bool is_v4_loopback(__be32 daddr)
{
	/* Check for 127.0.0.0/8 range, RFC3330. */
	return (daddr & bpf_htonl(0x7f000000)) == bpf_htonl(0x7f000000);
}

static __always_inline int get_pid() {
    return bpf_get_current_pid_tgid() >> 32;
}

static __always_inline int trace_udp(void *ctx, struct sock *sk, u64 size) {
    u16 family;
    bpf_probe_read_kernel(&family, sizeof(family), &sk->__sk_common.skc_family);

    if (family == AF_INET) {
        // Read dest port.
        __be16 dport;
        bpf_probe_read_kernel(&dport, sizeof(dport), &sk->__sk_common.skc_dport);
        if (bpf_ntohs(dport) != 53) {
            return 0;
        }

        struct dns_packet_t *pkt = bpf_map_lookup_elem(&dns_packet_map, DNS_PACKET_MAP_KEY);
        if (!pkt) {
            return 0;
        }

        struct sk_buff *skb = (struct sk_buff *)(sk);
        u8 pkt;
        bpf_skb_load_bytes(skb, 0, &pkt, sizeof(u8));
        bpf_perf_event_output(ctx, &dns_events, BPF_F_CURRENT_CPU, &pkt, sizeof(pkt));
    }

    // TODO: Add support for IPv6
    return 0;
}

SEC("kprobe/udp_recvmsg")
int BPF_KPROBE(udp_recvmsg, struct sock *sk, struct msghdr *msg, size_t size, int flags, int *addr_len)
{
    return trace_udp(ctx, sk, size);
}
