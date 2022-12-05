package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"net/http/pprof"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/castai/egressd/collector"
	"github.com/castai/egressd/conntrack"
	"github.com/castai/egressd/exporter"
	"github.com/castai/egressd/kube"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/flowcontrol"
)

var (
	kubeconfig        = flag.String("kubeconfig", "", "")
	conntrackMode     = flag.String("conntrack-mode", "nf", "")
	interval          = flag.Duration("interval", 5*time.Second, "")
	httpAddr          = flag.String("http-addr", ":6060", "")
	exportFileName    = flag.String("export-file", "/var/run/egressd/egressd.log", "Export file name")
	excludeNamespaces = flag.String("exclude-namespaces", "kube-system", "Exclude namespaces from collections")
)

// These should be set via `go build` during a release.
var (
	GitCommit = "undefined"
	GitRef    = "no-ref"
	Version   = "local"
)

func main() {
	flag.Parse()

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	ctx, shutdown := context.WithCancel(context.Background())
	defer shutdown()

	go func() {
		stopper := make(chan os.Signal, 1)
		signal.Notify(stopper, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
		<-stopper
		shutdown()
	}()

	go func() {
		mux := http.NewServeMux()
		addPprofHandlers(mux)
		_ = http.ListenAndServe(*httpAddr, mux) //nolint:gosec
	}()

	if err := run(ctx, log); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run(ctx context.Context, log logrus.FieldLogger) error {
	log.Infof("running egressd, version=%s, commit=%s, ref=%s", Version, GitCommit, GitRef)

	restconfig, err := retrieveKubeConfig(log, *kubeconfig)
	if err != nil {
		return err
	}
	restconfig.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(float32(25), 100)
	clientset, err := kubernetes.NewForConfig(restconfig)
	if err != nil {
		return err
	}

	kubeWatcher := kube.NewWatcher(clientset)
	var conntracker conntrack.Client
	switch *conntrackMode {
	case "nf":
		conntracker, err = conntrack.NewNetfilterClient(log)
	case "cilium":
		conntracker, err = conntrack.NewCiliumClient()
	default:
		return fmt.Errorf("unsupported conntract-mode %q", *conntrackMode)
	}
	defer conntracker.Close()
	if err != nil {
		return err
	}
	cfg := collector.Config{
		Interval:          *interval,
		NodeName:          os.Getenv("NODE_NAME"),
		ExcludeNamespaces: *excludeNamespaces,
		CacheItems:        20000,
	}
	coll := collector.New(cfg, log, kubeWatcher, conntracker)

	if *exportFileName != "" {
		export := exporter.New(exporter.Config{
			ExportFilename:      *exportFileName,
			ExportFileMaxSizeMB: 10,
			MaxBackups:          3,
			Compress:            false,
		}, log, coll)
		go func() {
			if err := export.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Errorf("exporter failed: %v", err)
			}
		}()
	}

	return coll.Start(ctx)
}

func retrieveKubeConfig(log logrus.FieldLogger, kubepath string) (*rest.Config, error) {
	if kubepath != "" {
		data, err := os.ReadFile(kubepath)
		if err != nil {
			return nil, fmt.Errorf("reading kubeconfig at %s: %w", kubepath, err)
		}
		restConfig, err := clientcmd.RESTConfigFromKubeConfig(data)
		if err != nil {
			return nil, fmt.Errorf("building rest config from kubeconfig at %s: %w", kubepath, err)
		}
		log.Debug("using kubeconfig from env variables")
		return restConfig, nil
	}

	inClusterConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}
	log.Debug("using in cluster kubeconfig")
	return inClusterConfig, nil
}

func addPprofHandlers(mux *http.ServeMux) {
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
}
