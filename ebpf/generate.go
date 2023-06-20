package ebpf

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang-14 -cflags $BPF_CFLAGS bpf ./c/egressd.c -- -I../headers
