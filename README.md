# egressd

Kubernetes aware network traffic monitoring.

## How it works

Egressd is Kubernetes DaemonSet. Pod running on each node collects conntrack records and adds Kubernetes context.
On Kubernetes clusters with Cilium CNI egressd hooks into eBPF maps located at /sys/fs/bpf
For other CNI it fetches conntrack records from /proc/sys/net/netfilter

## Development

Build and push docker image
```
IMAGE_TAG=my-tag make push-docker && k rollout restart ds egressd
```

Expose cilium socket via TCP. Not used by code, but could be useful in the feature.
```
socat TCP-LISTEN:9000,reuseaddr,fork UNIX-CONNECT:/var/run/cilium/cilium.sock
```
