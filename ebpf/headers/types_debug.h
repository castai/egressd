
//SEC("kprobe/udp_sendmsg")
//int BPF_KPROBE(tcp_sendmsg, struct sock *sk, struct msghdr *msg, size_t size) {
//    return 0;
//}
//
//SEC("kprobe/udp_recvmsg")
//int BPF_KPROBE(tcp_sendmsg, struct sock *sk, struct msghdr *msg, size_t size) {
//    return 0;
//}

/*
struct trace_event_raw_sys_exit {
        struct trace_entry         ent;
        long int                   id;
        long int                   ret;
        char                       __data[];
};
*/

/*
tcp:tcp_retransmit_skb: Traces retransmits. Useful for understanding network issues including congestion. Will be used by my tcpretrans tools instead of kprobes.
tcp:tcp_retransmit_synack: Tracing SYN/ACK retransmits. I imagine this could be used for a type of DoS detection (SYN flood triggering SYN/ACKs and then retransmits). This is separate from tcp:tcp_retransmit_skb because this event doesn't have an skb.
tcp:tcp_destroy_sock: Needed by any program that summarizes details in-memory for a TCP session, which would be keyed by the sock address. This probe can be used to know that the session has ended, so that sock address is about to be reused and any summarized data so far should be consumed and then deleted.
tcp:tcp_send_reset: This traces a RST send during a valid socket, to diagnose those type of issues.
tcp:tcp_receive_reset: Trace a RST receive.
tcp:tcp_probe: for TCP congestion window tracing, which also allowed an older TCP probe module to be deprecated and removed. This was added by Masami Hiramatsu, and merged in Linux 4.16.
sock:inet_sock_set_state: Can be used for many things. The tcplife tool is one, but also my tcpconnect and tcpaccept bcc tools can be converted to use this tracepoint. We could add separate tcp:tcp_connect and tcp:tcp_accept tracepoints (or tcp:tcp_active_open and tcp:tcp_passive_open), but sock:inet_sock_set_state can be used for this.
*/

/*
struct trace_entry {
        short unsigned int         type;
        unsigned char              flags;
        unsigned char              preempt_count;
        int                        pid;

};
struct trace_event_raw_sys_enter {
        struct trace_entry         ent;
        long int                   id;
        long unsigned int          args[6];
        char                       __data[];
};
*/

//struct trace_event_raw_sys_enter {
//        struct trace_entry         ent;
//        long int                   id;
//        long unsigned int          args[6];
//        char                       __data[];
//};

//struct trace_event_raw_tcp_event_sk_skb {
//        struct trace_entry         ent;                  /*     0     8 */
//        const void  *              skbaddr;              /*     8     8 */
//        const void  *              skaddr;               /*    16     8 */
//        int                        state;                /*    24     4 */
//        __u16                      sport;                /*    28     2 */
//        __u16                      dport;                /*    30     2 */
//        __u8                       saddr[4];             /*    32     4 */
//        __u8                       daddr[4];             /*    36     4 */
//        __u8                       saddr_v6[16];         /*    40    16 */
//        __u8                       daddr_v6[16];         /*    56    16 */
//        /* --- cacheline 1 boundary (64 bytes) was 8 bytes ago --- */
//        char                       __data[];             /*    72     0 */
//
//        /* size: 72, cachelines: 2, members: 11 */
//        /* last cacheline: 8 bytes */
//};

/*
struct in6_addr {
	union {
		__u8 u6_addr8[16];
		__be16 u6_addr16[8];
		__be32 u6_addr32[4];
	} in6_u;
};

struct sk_buff {
	union {
		struct {
			struct sk_buff *next;
			struct sk_buff *prev;
			union {
				struct net_device *dev;
				long unsigned int dev_scratch;
			};
		};
		struct rb_node rbnode;
		struct list_head list;
	};
	union {
		struct sock *sk;
		int ip_defrag_offset;
	};
	union {
		ktime_t tstamp;
		u64 skb_mstamp_ns;
	};
	char cb[48];
	union {
		struct {
			long unsigned int _skb_refdst;
			void (*destructor)(struct sk_buff *);
		};
		struct list_head tcp_tsorted_anchor;
	};
	long unsigned int _nfct;
	unsigned int len;
	unsigned int data_len;
	__u16 mac_len;
	__u16 hdr_len;
	__u16 queue_mapping;
	__u8 __cloned_offset[0];
	__u8 cloned: 1;
	__u8 nohdr: 1;
	__u8 fclone: 2;
	__u8 peeked: 1;
	__u8 head_frag: 1;
	__u8 pfmemalloc: 1;
	__u8 active_extensions;
	__u32 headers_start[0];
	__u8 __pkt_type_offset[0];
	__u8 pkt_type: 3;
	__u8 ignore_df: 1;
	__u8 nf_trace: 1;
	__u8 ip_summed: 2;
	__u8 ooo_okay: 1;
	__u8 l4_hash: 1;
	__u8 sw_hash: 1;
	__u8 wifi_acked_valid: 1;
	__u8 wifi_acked: 1;
	__u8 no_fcs: 1;
	__u8 encapsulation: 1;
	__u8 encap_hdr_csum: 1;
	__u8 csum_valid: 1;
	__u8 __pkt_vlan_present_offset[0];
	__u8 vlan_present: 1;
	__u8 csum_complete_sw: 1;
	__u8 csum_level: 2;
	__u8 csum_not_inet: 1;
	__u8 dst_pending_confirm: 1;
	__u8 ndisc_nodetype: 2;
	__u8 ipvs_property: 1;
	__u8 inner_protocol_type: 1;
	__u8 remcsum_offload: 1;
	__u8 offload_fwd_mark: 1;
	__u8 offload_l3_fwd_mark: 1;
	__u8 tc_skip_classify: 1;
	__u8 tc_at_ingress: 1;
	__u8 redirected: 1;
	__u8 from_ingress: 1;
	__u8 decrypted: 1;
	__u16 tc_index;
	union {
		__wsum csum;
		struct {
			__u16 csum_start;
			__u16 csum_offset;
		};
	};
	__u32 priority;
	int skb_iif;
	__u32 hash;
	__be16 vlan_proto;
	__u16 vlan_tci;
	union {
		unsigned int napi_id;
		unsigned int sender_cpu;
	};
	__u32 secmark;
	union {
		__u32 mark;
		__u32 reserved_tailroom;
	};
	union {
		__be16 inner_protocol;
		__u8 inner_ipproto;
	};
	__u16 inner_transport_header;
	__u16 inner_network_header;
	__u16 inner_mac_header;
	__be16 protocol;
	__u16 transport_header;
	__u16 network_header;
	__u16 mac_header;
	__u32 headers_end[0];
	sk_buff_data_t tail;
	sk_buff_data_t end;
	unsigned char *head;
	unsigned char *data;
	unsigned int truesize;
	refcount_t users;
	struct skb_ext *extensions;
};

struct sock {
	struct sock_common __sk_common;
	socket_lock_t sk_lock;
	atomic_t sk_drops;
	int sk_rcvlowat;
	struct sk_buff_head sk_error_queue;
	struct sk_buff *sk_rx_skb_cache;
	struct sk_buff_head sk_receive_queue;
	struct {
		atomic_t rmem_alloc;
		int len;
		struct sk_buff *head;
		struct sk_buff *tail;
	} sk_backlog;
	int sk_forward_alloc;
	unsigned int sk_ll_usec;
	unsigned int sk_napi_id;
	int sk_rcvbuf;
	struct sk_filter *sk_filter;
	union {
		struct socket_wq *sk_wq;
		struct socket_wq *sk_wq_raw;
	};
	struct xfrm_policy *sk_policy[2];
	struct dst_entry *sk_rx_dst;
	struct dst_entry *sk_dst_cache;
	atomic_t sk_omem_alloc;
	int sk_sndbuf;
	int sk_wmem_queued;
	refcount_t sk_wmem_alloc;
	long unsigned int sk_tsq_flags;
	union {
		struct sk_buff *sk_send_head;
		struct rb_root tcp_rtx_queue;
	};
	struct sk_buff *sk_tx_skb_cache;
	struct sk_buff_head sk_write_queue;
	__s32 sk_peek_off;
	int sk_write_pending;
	__u32 sk_dst_pending_confirm;
	u32 sk_pacing_status;
	long int sk_sndtimeo;
	struct timer_list sk_timer;
	__u32 sk_priority;
	__u32 sk_mark;
	long unsigned int sk_pacing_rate;
	long unsigned int sk_max_pacing_rate;
	struct page_frag sk_frag;
	netdev_features_t sk_route_caps;
	netdev_features_t sk_route_nocaps;
	netdev_features_t sk_route_forced_caps;
	int sk_gso_type;
	unsigned int sk_gso_max_size;
	gfp_t sk_allocation;
	__u32 sk_txhash;
	u8 sk_padding: 1;
	u8 sk_kern_sock: 1;
	u8 sk_no_check_tx: 1;
	u8 sk_no_check_rx: 1;
	u8 sk_userlocks: 4;
	u8 sk_pacing_shift;
	u16 sk_type;
	u16 sk_protocol;
	u16 sk_gso_max_segs;
	long unsigned int sk_lingertime;
	struct proto *sk_prot_creator;
	rwlock_t sk_callback_lock;
	int sk_err;
	int sk_err_soft;
	u32 sk_ack_backlog;
	u32 sk_max_ack_backlog;
	kuid_t sk_uid;
	struct pid *sk_peer_pid;
	const struct cred *sk_peer_cred;
	long int sk_rcvtimeo;
	ktime_t sk_stamp;
	u16 sk_tsflags;
	u8 sk_shutdown;
	u32 sk_tskey;
	atomic_t sk_zckey;
	u8 sk_clockid;
	u8 sk_txtime_deadline_mode: 1;
	u8 sk_txtime_report_errors: 1;
	u8 sk_txtime_unused: 6;
	struct socket *sk_socket;
	void *sk_user_data;
	void *sk_security;
	struct sock_cgroup_data sk_cgrp_data;
	struct mem_cgroup *sk_memcg;
	void (*sk_state_change)(struct sock *);
	void (*sk_data_ready)(struct sock *);
	void (*sk_write_space)(struct sock *);
	void (*sk_error_report)(struct sock *);
	int (*sk_backlog_rcv)(struct sock *, struct sk_buff *);
	struct sk_buff * (*sk_validate_xmit_skb)(struct sock *, struct net_device *, struct sk_buff *);
	void (*sk_destruct)(struct sock *);
	struct sock_reuseport *sk_reuseport_cb;
	struct bpf_sk_storage *sk_bpf_storage;
	struct callback_head sk_rcu;
};

struct sock_common {
	union {
		__addrpair skc_addrpair;
		struct {
			__be32 skc_daddr;
			__be32 skc_rcv_saddr;
		};
	};
	union {
		unsigned int skc_hash;
		__u16 skc_u16hashes[2];
	};
	union {
		__portpair skc_portpair;
		struct {
			__be16 skc_dport;
			__u16 skc_num;
		};
	};
	short unsigned int skc_family;
	volatile unsigned char skc_state;
	unsigned char skc_reuse: 4;
	unsigned char skc_reuseport: 1;
	unsigned char skc_ipv6only: 1;
	unsigned char skc_net_refcnt: 1;
	int skc_bound_dev_if;
	union {
		struct hlist_node skc_bind_node;
		struct hlist_node skc_portaddr_node;
	};
	struct proto *skc_prot;
	possible_net_t skc_net;
	struct in6_addr skc_v6_daddr;
	struct in6_addr skc_v6_rcv_saddr;
	atomic64_t skc_cookie;
	union {
		long unsigned int skc_flags;
		struct sock *skc_listener;
		struct inet_timewait_death_row *skc_tw_dr;
	};
	int skc_dontcopy_begin[0];
	union {
		struct hlist_node skc_node;
		struct hlist_nulls_node skc_nulls_node;
	};
	short unsigned int skc_tx_queue_mapping;
	short unsigned int skc_rx_queue_mapping;
	union {
		int skc_incoming_cpu;
		u32 skc_rcv_wnd;
		u32 skc_tw_rcv_nxt;
	};
	refcount_t skc_refcnt;
	int skc_dontcopy_end[0];
	union {
		u32 skc_rxhash;
		u32 skc_window_clamp;
		u32 skc_tw_snd_nxt;
	};
};


struct tcp_sock {
	struct inet_connection_sock inet_conn;
	u16 tcp_header_len;
	u16 gso_segs;
	__be32 pred_flags;
	u64 bytes_received;
	u32 segs_in;
	u32 data_segs_in;
	u32 rcv_nxt;
	u32 copied_seq;
	u32 rcv_wup;
	u32 snd_nxt;
	u32 segs_out;
	u32 data_segs_out;
	u64 bytes_sent;
	u64 bytes_acked;
	u32 dsack_dups;
	u32 snd_una;
	u32 snd_sml;
	u32 rcv_tstamp;
	u32 lsndtime;
	u32 last_oow_ack_time;
	u32 compressed_ack_rcv_nxt;
	u32 tsoffset;
	struct list_head tsq_node;
	struct list_head tsorted_sent_queue;
	u32 snd_wl1;
	u32 snd_wnd;
	u32 max_window;
	u32 mss_cache;
	u32 window_clamp;
	u32 rcv_ssthresh;
	struct tcp_rack rack;
	u16 advmss;
	u8 compressed_ack;
	u8 dup_ack_counter: 2;
	u8 tlp_retrans: 1;
	u8 unused: 5;
	u32 chrono_start;
	u32 chrono_stat[3];
	u8 chrono_type: 2;
	u8 rate_app_limited: 1;
	u8 fastopen_connect: 1;
	u8 fastopen_no_cookie: 1;
	u8 is_sack_reneg: 1;
	u8 fastopen_client_fail: 2;
	u8 nonagle: 4;
	u8 thin_lto: 1;
	u8 recvmsg_inq: 1;
	u8 repair: 1;
	u8 frto: 1;
	u8 repair_queue;
	u8 syn_data: 1;
	u8 syn_fastopen: 1;
	u8 syn_fastopen_exp: 1;
	u8 syn_fastopen_ch: 1;
	u8 syn_data_acked: 1;
	u8 save_syn: 1;
	u8 is_cwnd_limited: 1;
	u8 syn_smc: 1;
	u32 tlp_high_seq;
	u32 tcp_tx_delay;
	u64 tcp_wstamp_ns;
	u64 tcp_clock_cache;
	u64 tcp_mstamp;
	u32 srtt_us;
	u32 mdev_us;
	u32 mdev_max_us;
	u32 rttvar_us;
	u32 rtt_seq;
	struct minmax rtt_min;
	u32 packets_out;
	u32 retrans_out;
	u32 max_packets_out;
	u32 max_packets_seq;
	u16 urg_data;
	u8 ecn_flags;
	u8 keepalive_probes;
	u32 reordering;
	u32 reord_seen;
	u32 snd_up;
	struct tcp_options_received rx_opt;
	u32 snd_ssthresh;
	u32 snd_cwnd;
	u32 snd_cwnd_cnt;
	u32 snd_cwnd_clamp;
	u32 snd_cwnd_used;
	u32 snd_cwnd_stamp;
	u32 prior_cwnd;
	u32 prr_delivered;
	u32 prr_out;
	u32 delivered;
	u32 delivered_ce;
	u32 lost;
	u32 app_limited;
	u64 first_tx_mstamp;
	u64 delivered_mstamp;
	u32 rate_delivered;
	u32 rate_interval_us;
	u32 rcv_wnd;
	u32 write_seq;
	u32 notsent_lowat;
	u32 pushed_seq;
	u32 lost_out;
	u32 sacked_out;
	struct hrtimer pacing_timer;
	struct hrtimer compressed_ack_timer;
	struct sk_buff *lost_skb_hint;
	struct sk_buff *retransmit_skb_hint;
	struct rb_root out_of_order_queue;
	struct sk_buff *ooo_last_skb;
	struct tcp_sack_block duplicate_sack[1];
	struct tcp_sack_block selective_acks[4];
	struct tcp_sack_block recv_sack_cache[4];
	struct sk_buff *highest_sack;
	int lost_cnt_hint;
	u32 prior_ssthresh;
	u32 high_seq;
	u32 retrans_stamp;
	u32 undo_marker;
	int undo_retrans;
	u64 bytes_retrans;
	u32 total_retrans;
	u32 urg_seq;
	unsigned int keepalive_time;
	unsigned int keepalive_intvl;
	int linger2;
	u8 bpf_sock_ops_cb_flags;
	u16 timeout_rehash;
	u32 rcv_ooopack;
	u32 rcv_rtt_last_tsecr;
	struct {
		u32 rtt_us;
		u32 seq;
		u64 time;
	} rcv_rtt_est;
	struct {
		u32 space;
		u32 seq;
		u64 time;
	} rcvq_space;
	struct {
		u32 probe_seq_start;
		u32 probe_seq_end;
	} mtu_probe;
	u32 mtu_info;
	bool is_mptcp;
	const struct tcp_sock_af_ops *af_specific;
	struct tcp_md5sig_info *md5sig_info;
	struct tcp_fastopen_request *fastopen_req;
	struct request_sock *fastopen_rsk;
	u32 *saved_syn;
};


union tcp_word_hdr {
        struct tcphdr hdr;
        __be32 words[5];
};

struct tcphdr {
        __be16 source;
        __be16 dest;
        __be32 seq;
        __be32 ack_seq;
        __u16 res1: 4;
        __u16 doff: 4;
        __u16 fin: 1;
        __u16 syn: 1;
        __u16 rst: 1;
        __u16 psh: 1;
        __u16 ack: 1;
        __u16 urg: 1;
        __u16 ece: 1;
        __u16 cwr: 1;
        __be16 window;
        __sum16 check;
        __be16 urg_ptr;
};

struct iphdr {
        __u8 ihl: 4;
        __u8 version: 4;
        __u8 tos;
        __be16 tot_len;
        __be16 id;
        __be16 frag_off;
        __u8 ttl;
        __u8 protocol;
        __sum16 check;
        __be32 saddr;
        __be32 daddr;
};

struct udphdr {
        __be16 source;
        __be16 dest;
        __be16 len;
        __sum16 check;
};


struct __sk_buff {
        __u32 len;
        __u32 pkt_type;
        __u32 mark;
        __u32 queue_mapping;
        __u32 protocol;
        __u32 vlan_present;
        __u32 vlan_tci;
        __u32 vlan_proto;
        __u32 priority;
        __u32 ingress_ifindex;
        __u32 ifindex;
        __u32 tc_index;
        __u32 cb[5];
        __u32 hash;
        __u32 tc_classid;
        __u32 data;
        __u32 data_end;
        __u32 napi_id;
        __u32 family;
        __u32 remote_ip4;
        __u32 local_ip4;
        __u32 remote_ip6[4];
        __u32 local_ip6[4];
        __u32 remote_port;
        __u32 local_port;
        __u32 data_meta;
        union {
                struct bpf_flow_keys *flow_keys;
        };
        __u64 tstamp;
        __u32 wire_len;
        __u32 gso_segs;
        union {
                struct bpf_sock *sk;
        };
};




*/