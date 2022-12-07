# egressd

Kubernetes aware network traffic monitoring.

## How it works

* DaemonSet pod starts on each node.
* Conntrack entries are fetched for pods running on each at configured interval (5 seconds by default).
  * If Cilium is used then conntrack records are fetched from eBPF maps located at host /sys/fs/bpf. These maps are created by Cilium.
  * If Linux Netfilter Conntrack module is used then Netlink is used to get these records.
* Records are reduced by source IP, destination, IP and protocol.
* Kubernetes context is added including source and destination pods, nodes, node zones, ips.
* Logs are written to logs file. This allows to setup logs processing tools for export to other systems.

## Limitations and feature improvements

* Pods running on host network are not tracked. Conntrack records do not have process ID which makes it hard to map host network ips into actual pods. In the feature we should track all listening and allocated ports for processes and coordinate between egressd instances.
* Docker image is 200MB due to use of Go github.com/cilium/cilium packages which requires bpftool and libelf. In the feature this can be replaced with github.com/cilium/ebpf for reading eBPF maps.
* When running with Linux conntrack, network flows are tracked only if stats are enabled. Init container enables these stats, but you may need to reload already running workloads manually.
* Currently each pod runs on hostNetwork and priviledged mode. For linux conntrack this can be removed by parsing mounted `/proc/net/nf_conntrack` instead of using netlink. For cilium priviledged mode is still needed to read eBPF maps.

## Example

![alt text](https://github.com/castai/egressd/blob/94dab4aab2179a75f695c596b275c99ae4dfe837/examples/vector/dashboard.png)

See https://github.com/castai/egressd/tree/main/examples/vector


## Development

Build and push docker image
```
IMAGE_TAG=my-tag make push-docker && k rollout restart ds egressd
```

Expose cilium socket via TCP. Not used by code, but could be useful in the feature.
```
socat TCP-LISTEN:9000,reuseaddr,fork UNIX-CONNECT:/var/run/cilium/cilium.sock
```
