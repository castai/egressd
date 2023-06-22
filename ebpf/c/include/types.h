#include <vmlinux.h>
#include <vmlinux_missing.h>

typedef struct task_context {
    u64 start_time; // thread's start time
    u64 cgroup_id;
    u32 pid;       // PID as in the userspace term
    u32 tid;       // TID as in the userspace term
    u32 ppid;      // Parent PID as in the userspace term
    u32 host_pid;  // PID in host pid namespace
    u32 host_tid;  // TID in host pid namespace
    u32 host_ppid; // Parent PID in host pid namespace
    u32 uid;
    u32 mnt_id;
    u32 pid_id;
    char comm[TASK_COMM_LEN];
    char uts_name[TASK_COMM_LEN];
    u32 flags;
} task_context_t;

typedef struct event_context {
    u64 ts; // Timestamp
    task_context_t task;
    u32 eventid;
    s32 syscall; // The syscall which triggered the event
    u64 matched_policies;
    s64 retval;
    u32 stack_id;
    u16 processor_id; // The ID of the processor which processed the event
    u8 argnum;
} event_context_t;

typedef struct net_event_contextmd {
    u8 submit; // keeping this for a TODO (check should_submit_net_event)
    u32 header_size;
    u8 captured;
} __attribute__((__packed__)) net_event_contextmd_t;

typedef struct net_event_context {
    event_context_t eventctx;
    struct { // event arguments (needs packing), use anonymous struct to ...
        u8 index0;
        u32 bytes;
        // ... (payload sent by bpf_perf_event_output)
    } __attribute__((__packed__)); // ... avoid address-of-packed-member warns
    // members bellow this point are metadata (not part of event to be sent)
    net_event_contextmd_t md;
} __attribute__((__packed__)) net_event_context_t;