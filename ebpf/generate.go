package ebpf

//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang-14 -target arm64 -cflags $BPF_CFLAGS bpf ./c/egressd.c -- -I./c/include
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc clang-14 -target amd64 -cflags $BPF_CFLAGS bpf ./c/egressd.c -- -I./c/include
