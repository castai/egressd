#!/usr/bin/env bash

# Version of libbpf to fetch headers from
LIBBPF_VERSION=0.6.1

# The headers we want
prefix=libbpf-"$LIBBPF_VERSION"
headers=(
    "$prefix"/LICENSE.BSD-2-Clause
    "$prefix"/src/bpf_endian.h
    "$prefix"/src/bpf_helper_defs.h
    "$prefix"/src/bpf_helpers.h
    "$prefix"/src/bpf_tracing.h
)

# Fetch libbpf release and extract the desired headers
curl -sL "https://github.com/libbpf/libbpf/archive/refs/tags/v${LIBBPF_VERSION}.tar.gz" | \
    tar -xz --xform='s#.*/##' "${headers[@]}"

# Update vmlinux.h
# To update from local linux VM
# bpftool btf dump file /sys/kernel/btf/vmlinux format c > vmlinux.h
# To Update from cloud instance.
#gcloud compute scp --zone "us-central1-c"  gke-am-test-default-pool-6df4fd07-x6qj:/vmlinux.h vmlinux.h