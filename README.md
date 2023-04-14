# egressd

Kubernetes aware network traffic monitoring.

## How it works

* DaemonSet pod starts on each node.
* Conntrack entries are fetched for pods running on each at configured interval (5 seconds by default).
  * If Cilium is used then conntrack records are fetched from eBPF maps located at host /sys/fs/bpf. These maps are created by Cilium.
  * If Linux Netfilter Conntrack module is used then Netlink is used to get these records.
* Records are reduced by source IP, destination, IP and protocol.
* Kubernetes context is added including source and destination pods, nodes, node zones, ips.
* Exporter can export logs to http or prometheus.

## Install

```sh
helm repo add castai-helm https://castai.github.io/helm-charts
helm repo update castai-helm

helm upgrade --install castai-egressd castai-helm/egressd -n castai-agent \
  --set castai.apiKey=<your-api-token> \
  --set castai.clusterID=<your-cluster-id>
```

### Development

Start all components + test grafana,promtheus in tilt local k8s cluster.
```
tilt up
```

Run e2e tests locally
```
KIND_CONTEXT=tilt IMAGE_TAG=local ./e2e/run.sh
```
