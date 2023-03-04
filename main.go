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
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/flowcontrol"
)

var (
	kubeconfig        = flag.String("kubeconfig", "", "")
	logLevel          = flag.String("log-level", logrus.InfoLevel.String(), "Log level")
	readInterval      = flag.Duration("read-interval", 5*time.Second, "Interval of time between reads of conntrack entry on the node")
	flushInterval     = flag.Duration("flush-interval", 60*time.Second, "Interval of time for flushing pod network cache")
	cleanupInterval   = flag.Duration("cleanup-interval", 123*time.Second, "Interval of time for cleanup cached conntrack entries")
	httpAddr          = flag.String("http-addr", ":6060", "")
	exportMode        = flag.String("export-mode", "http", "Export mode. Available values: http,file")
	exportHTTPAddr    = flag.String("export-http-addr", "http://egressd-aggregator:6000", "Export to vector aggregator http source")
	exportFileName    = flag.String("export-file", "/var/run/egressd/egressd.log", "Export file name")
	excludeNamespaces = flag.String("exclude-namespaces", "kube-system", "Exclude namespaces from collections")
	metricBufferSize  = flag.Int("metric-buffer-size", 10000, "Amount of entries that metrics buffer allows storing before blocking")
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
	lvl, err := logrus.ParseLevel(*logLevel)
	if err != nil {
		log.Fatal(err)
	}
	log.SetLevel(lvl)

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
		mux.Handle("/metrics", promhttp.Handler())
		_ = http.ListenAndServe(*httpAddr, mux) //nolint:gosec
	}()

	if err := run(ctx, log); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run(ctx context.Context, log logrus.FieldLogger) error {
	log.Infof("running egressd, version=%s, commit=%s, ref=%s, read-interval=%s, flush-interval=%s", Version, GitCommit, GitRef, *readInterval, *flushInterval)

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

	ciliumAvailable := conntrack.CiliumAvailable()
	if ciliumAvailable {
		conntracker, err = conntrack.NewCiliumClient()
	} else {
		conntracker, err = conntrack.NewNetfilterClient(log)
	}

	if err != nil {
		return err
	}
	dnsStore, err := collector.NewEBPFDNSStore(ctx, log)
	if err != nil {
		return err
	}
	cfg := collector.Config{
		ReadInterval:      *readInterval,
		FlushInterval:     *flushInterval,
		CleanupInterval:   *cleanupInterval,
		NodeName:          os.Getenv("NODE_NAME"),
		ExcludeNamespaces: *excludeNamespaces,
		MetricBufferSize:  *metricBufferSize,
	}
	coll := collector.New(
		cfg,
		log,
		kubeWatcher,
		conntracker,
		collector.CurrentTimeGetter(),
		dnsStore,
	)

	switch *exportMode {
	case "http":
		export := exporter.NewHTTPExporter(exporter.HTTPConfig{Addr: *exportHTTPAddr}, log, coll)
		go func() {
			if err := export.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Errorf("exporter failed: %v", err)
			}
		}()
	case "file":
		if *exportFileName != "" {
			export := exporter.NewFileExporter(exporter.FileConfig{
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
		} else {
			return errors.New("export file name is empty")
		}
	default:
		return fmt.Errorf("export mode %q is not supported", *exportMode)
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
