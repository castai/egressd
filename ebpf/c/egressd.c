#include <vmlinux.h>
#include <vmlinux_flavors.h>
#include <vmlinux_missing.h>

#include <bpf_helpers.h>
#include <bpf_core_read.h>
#include <bpf_endian.h>

#include <maps.h>
#include <common.h>
#include <types.h>

char __license[] SEC("license") = "Dual MIT/GPL";

#define UDP_PORT_DNS 53

BPF_PERF_OUTPUT(events, 1024); // Events output map.

statfunc int get_pid() {
    return bpf_get_current_pid_tgid() >> 32;
}

statfunc u32 cgroup_skb_generic(struct __sk_buff *ctx)
{
    switch (ctx->family) {
        case PF_INET:
            break;
        default:
            return 1;
    }

    struct bpf_sock *sk = ctx->sk;
    if (!sk)
        return 1;

    sk = bpf_sk_fullsock(sk);
    if (!sk)
        return 1;

    nethdrs hdrs = {0}, *nethdrs = &hdrs;

    void *dest;
    u32 size = 0;
    dest = &nethdrs->iphdrs.iphdr;
    size = get_type_size(struct iphdr);

    // Load L3 protocol headers.
    if (bpf_skb_load_bytes_relative(ctx, 0, dest, size, 1)) {
        return 1;
    }

    if (nethdrs->iphdrs.iphdr.version != 4)
        return 1;

    u32 ihl = nethdrs->iphdrs.iphdr.ihl;
    if (ihl > 5) { // re-read IPv4 header if needed
        size -= get_type_size(struct iphdr);
        size += ihl * 4;
        bpf_skb_load_bytes_relative(ctx, 0, dest, size, 1);
    }

    switch (nethdrs->iphdrs.iphdr.protocol) {
        case IPPROTO_UDP:
            break;
        // TODO: Add support for DNS over TCP.
        default:
            return 1;
    }

    u32 prev_hdr_size = size;
    size = get_type_size(struct udphdr);
    dest = &nethdrs->protohdrs.udphdr;

    if (!dest)
        return 1;

    // Load L4 headers.
    if (size) {
        if (bpf_skb_load_bytes_relative(ctx,
                                        prev_hdr_size,
                                        dest,
                                        size,
                                        BPF_HDR_START_NET))
            return 1;
    }

    u16 src_port = bpf_ntohs(nethdrs->protohdrs.udphdr.source);
    u16 dst_port = bpf_ntohs(nethdrs->protohdrs.udphdr.dest);

    switch (src_port < dst_port ? src_port : dst_port) {
        case UDP_PORT_DNS:
            break;
        default:
            return 1;
    }

    net_event_context_t neteventctx = {};
    neteventctx.bytes = ctx->len;

    u64 flags = BPF_F_CURRENT_CPU;
    u32 data_size = ctx->len;
    flags |= (u64) data_size << 32;

    bpf_perf_event_output(ctx,
                          &events,
                          flags,
                          &neteventctx,
                          sizeof(net_event_context_t));

    return 1;
}

SEC("cgroup_skb/ingress")
int cgroup_ingress(struct __sk_buff *skb) {
	return cgroup_skb_generic(skb);
}
