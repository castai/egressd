package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/sirupsen/logrus"

	"github.com/castai/egressd/ebpf"
)

/*
curl -L -O https://github.com/containerd/containerd/releases/download/v1.7.2/containerd-1.7.2-linux-amd64.tar.gz
tar Cxzvf /usr/local ./containerd-1.7.2-linux-amd64.tar.gz
curl -L -O https://github.com/opencontainers/runc/releases/download/v1.1.7/runc.amd64
install -m 755 runc.amd64 /usr/local/sbin/runc

curl -O -L https://github.com/containerd/nerdctl/releases/download/v1.4.0/nerdctl-1.4.0-linux-amd64.tar.gz
tar Cxzvf /usr/local/bin ./nerdctl-1.4.0-linux-amd64.tar.gz

curl -O -L https://github.com/containernetworking/plugins/releases/download/v1.3.0/cni-plugins-linux-amd64-v1.3.0.tgz
mkdir -p /opt/cni/bin
tar Cxzvf /opt/cni/bin cni-plugins-linux-amd64-v1.3.0.tgz

nerdctl run -d -p 8080:80 --name nginx nginx:alpine
*/
func main() {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	tr := ebpf.NewTracer(log)
	if err := tr.Run(ctx); err != nil {
		log.Fatalf("failed: %v", err)
	}
}
