# egressd

Kubernetes aware network traffic monitoring.

### How it works

* DaemonSet pod starts on each node.
* Conntrack entries are fetched for pods running on each at configured interval (5 seconds by default).
  * If Cilium is used then conntrack records are fetched from eBPF maps located at host /sys/fs/bpf. These maps are created by Cilium.
  * If Linux Netfilter Conntrack module is used then Netlink is used to get these records.
* Records are reduced by source IP, destination, IP and protocol.
* Kubernetes context is added including source and destination pods, nodes, node zones, ips.
* Exporter can export logs to http or prometheus.


### Install

**Install demo with preconfigured grafana and prometheus metrics.**
```
curl -fsSL https://raw.githubusercontent.com/castai/egressd/master/demo.sh | bash
```

**Expose grafana locally**
```sh
 kubectl port-forward svc/egressd-grafana 8080:80 -n egressd
```
Example dashboard available at http://localhost:8080/d/egressd/egressd
Metrics should be visible after few minutes.

![Dashboard](https://raw.githubusercontent.com/castai/egressd/main/egress.png)


**(Optionally) Install demo onlineboutique eshop**

If you want to test egressd on empty cluster.
```sh
helm upgrade --install onlineboutique oci://us-docker.pkg.dev/online-boutique-ci/charts/onlineboutique -n demo --create-namespace
```

### Development

Start all components + test grafana,promtheus in tilt local k8s cluster.
```
tilt up
```

### Release procedure (with automatic release notes)

Head to the [GitHub new release page](https://github.com/castai/egressd/releases/new), create a new tag at the top, and click `Generate Release Notes` at the middle-right.
![image](https://user-images.githubusercontent.com/571022/174777789-2d7d646d-714d-42da-8c66-a6ed407b4440.png)

Run e2e tests locally
```
KIND_CONTEXT=tilt IMAGE_TAG=local ./e2e/run.sh
```
