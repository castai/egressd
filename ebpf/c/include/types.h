#include <vmlinux.h>
#include <vmlinux_missing.h>

typedef struct event_context {
    u32 pid;
    u32 eventid;
} event_context_t;

typedef struct net_event_context {
    //event_context_t eventctx; Context not used right now.
    struct { // event arguments (needs packing), use anonymous struct to ...
        u32 bytes;
        // ... (payload sent by bpf_perf_event_output)
    } __attribute__((__packed__)); // ... avoid address-of-packed-member warns
} __attribute__((__packed__)) net_event_context_t;

typedef union iphdrs_t {
    struct iphdr iphdr;
    struct ipv6hdr ipv6hdr;
} iphdrs;

// NOTE: proto header structs need full type in vmlinux.h (for correct skb copy)

typedef union protohdrs_t {
    struct tcphdr tcphdr;
    struct udphdr udphdr;
    struct icmphdr icmphdr;
    struct icmp6hdr icmp6hdr;
    union {
        u8 tcp_extra[40]; // data offset might set it up to 60 bytes
    };
} protohdrs;

typedef struct nethdrs_t {
    iphdrs iphdrs;
    protohdrs protohdrs;
} nethdrs;