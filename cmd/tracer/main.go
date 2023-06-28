package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/sirupsen/logrus"

	"github.com/castai/egressd/ebpf"
)

// This is example program for ebpf dns tracer.
func main() {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	tr := ebpf.NewTracer(log, ebpf.Config{
		QueueSize: 1000,
		// Custom path should be used only for testing purposes in case there is no btf (local docker).
		// In prod do not set this and enable tracer only if ebpf.IsKernelBTFAvailable returns true.
		//CustomBTFFilePath: "/app/cmd/tracer/5.8.0-63-generic.btf",
	})
	errc := make(chan error, 1)
	go func() {
		errc <- tr.Run(ctx)
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case e := <-tr.Events():
			fmt.Println(e)
		case err := <-errc:
			log.Error(err)
		}
	}
}
