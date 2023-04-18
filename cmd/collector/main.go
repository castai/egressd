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

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/flowcontrol"

	"github.com/castai/egressd/collector"
	"github.com/castai/egressd/conntrack"
	"github.com/castai/egressd/kube"
)

var (
	kubeconfig        = flag.String("kubeconfig", "", "")
	logLevel          = flag.String("log-level", logrus.InfoLevel.String(), "Log level")
	readInterval      = flag.Duration("read-interval", 5*time.Second, "Interval of time between reads of conntrack entry on the node")
	cleanupInterval   = flag.Duration("cleanup-interval", 120*time.Second, "Interval of time for cleanup cached conntrack entries")
	httpListenPort    = flag.Int("http-listen-port", 8008, "HTTP server listen port")
	excludeNamespaces = flag.String("exclude-namespaces", "kube-system", "Exclude namespaces from collections")
	dumpCT            = flag.Bool("dump-ct", false, "Only dump connection tracking entries to stdout and exit")
	ciliumClockSource = flag.String("cilium-clock-source", string(conntrack.ClockSourceJiffies), "Kernel clock source used in cilium (jiffies or ktime)")
	groupPublicIPs    = flag.Bool("group-public-ips", false, "Group public ips destinations as 0.0.0.0")
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

	if *dumpCT {
		if err := dumpConntrack(log); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := run(log); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run(log logrus.FieldLogger) error {
	restconfig, err := retrieveKubeConfig(log, *kubeconfig)
	if err != nil {
		return err
	}
	restconfig.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(float32(25), 100)
	clientset, err := kubernetes.NewForConfig(restconfig)
	if err != nil {
		return err
	}

	// Setup shared informer indexers.
	informersFactory := informers.NewSharedInformerFactoryWithOptions(clientset, 30*time.Second, informers.WithTweakListOptions(func(options *metav1.ListOptions) {
		options.FieldSelector = "spec.nodeName=" + os.Getenv("NODE_NAME")
	}))
	podsInformer := informersFactory.Core().V1().Pods().Informer()
	podsByNodeCache := kube.NewPodsByNodeCache(podsInformer)
	informersFactory.Start(wait.NeverStop)
	informersFactory.WaitForCacheSync(wait.NeverStop)

	var conntracker conntrack.Client
	ciliumAvailable := conntrack.CiliumAvailable()
	if ciliumAvailable {
		conntracker, err = conntrack.NewCiliumClient(log, conntrack.ClockSource(*ciliumClockSource))
	} else {
		conntracker, err = conntrack.NewNetfilterClient(log)
	}

	if err != nil {
		return err
	}
	cfg := collector.Config{
		ReadInterval:      *readInterval,
		CleanupInterval:   *cleanupInterval,
		NodeName:          os.Getenv("NODE_NAME"),
		ExcludeNamespaces: *excludeNamespaces,
		GroupPublicIPs:    *groupPublicIPs,
	}
	coll := collector.New(
		cfg,
		log,
		podsByNodeCache,
		conntracker,
		collector.CurrentTimeGetter(),
	)

	mux := http.NewServeMux()
	addPprofHandlers(mux)
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", healthHandler)
	mux.HandleFunc("/api/v1/raw-network-metrics", coll.GetRawNetworkMetricsHandler)

	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", *httpListenPort),
		Handler:           mux,
		ReadHeaderTimeout: 3 * time.Second,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		stopper := make(chan os.Signal, 1)
		signal.Notify(stopper, os.Interrupt, syscall.SIGTERM, syscall.SIGINT)
		<-stopper

		// Stop http server.
		ctx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Errorf("http server shutdown: %v", err)
		}

		// Cancel context for other components like collector.
		cancel()
	}()

	go func() {
		if err := coll.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Errorf("starting collector: %v", err)
		}
	}()

	log.Infof("running egressd, version=%s, commit=%s, ref=%s, read-interval=%s", Version, GitCommit, GitRef, *readInterval)

	return srv.ListenAndServe()
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

func healthHandler(w http.ResponseWriter, req *http.Request) {
	_, _ = w.Write([]byte("Ok"))
}

func dumpConntrack(log logrus.FieldLogger) error {
	now := time.Now().UTC()
	var err error
	var conntracker conntrack.Client
	ciliumAvailable := conntrack.CiliumAvailable()
	if ciliumAvailable {
		conntracker, err = conntrack.NewCiliumClient(log, conntrack.ClockSource(*ciliumClockSource))
	} else {
		conntracker, err = conntrack.NewNetfilterClient(log)
	}
	if err != nil {
		return err
	}
	entries, err := conntracker.ListEntries(conntrack.All())
	if err != nil {
		return err
	}
	for _, e := range entries {
		fmt.Printf("proto=%d src=%s dst=%s tx_bytes=%d rx_bytes=%d expires=%v\n", e.Proto, e.Src, e.Dst, e.TxBytes, e.RxBytes, e.Lifetime.Sub(now))
	}
	return nil
}
