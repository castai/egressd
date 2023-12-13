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
	"github.com/castai/egressd/dns"
	"github.com/castai/egressd/ebpf"
	"github.com/castai/egressd/kube"
)

var (
	kubeconfig             = flag.String("kubeconfig", "", "")
	logLevel               = flag.String("log-level", logrus.InfoLevel.String(), "Log level")
	readInterval           = flag.Duration("read-interval", 5*time.Second, "Interval of time between reads of conntrack entry on the node")
	cleanupInterval        = flag.Duration("cleanup-interval", 120*time.Second, "Interval of time for cleanup cached conntrack entries")
	httpListenPort         = flag.Int("http-listen-port", 8008, "HTTP server listen port")
	hostPid                = flag.Bool("host-pid", false, "Use host pid")
	excludeNamespaces      = flag.String("exclude-namespaces", "kube-system", "Exclude namespaces from collections")
	dumpCT                 = flag.Bool("dump-ct", false, "Only dump connection tracking entries to stdout and exit")
	ciliumClockSource      = flag.String("cilium-clock-source", string(conntrack.ClockSourceJiffies), "Kernel clock source used in cilium (jiffies or ktime)")
	groupPublicIPs         = flag.Bool("group-public-ips", false, "Group public ips destinations as 0.0.0.0")
	sendTrafficDelta       = flag.Bool("send-traffic-delta", false, "Send traffic delta between reads of conntrack entry. Traffic counter is sent by default")
	ebpfDNSTracerEnabled   = flag.Bool("ebpf-dns-tracer-enabled", true, "Enable DNS tracer using eBPF")
	ebpfDNSTracerQueueSize = flag.Int("ebpf-dns-tracer-queue-size", 1000, "Size of the queue for DNS tracer")
	// Kubernetes requires container to run in privileged mode if Bidirectional mount is used.
	// Actually it needs only SYS_ADMIN but see this issue See https://github.com/kubernetes/kubernetes/pull/117812
	initMode     = flag.Bool("init", false, "Run in init mode")
	initCgroupv2 = flag.Bool("init-cgroupv2", false, "Mount cgroup v2 if needed")

	// Explicitly allow to set conntrack client mode.
	ctMode = flag.String("ct-mode", "", "Explicitly set conntract mode (netfilter,cilium)")
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

	if *initMode {
		if err := runInit(log); err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := run(log); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run(log logrus.FieldLogger) error {
	log.Infof("starting egressd, version=%s, commit=%s, ref=%s, read-interval=%s", Version, GitCommit, GitRef, *readInterval)

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
	ciliumAvailable := conntrack.CiliumAvailable("")
	if ciliumAvailable {
		log.Info("using cilium conntrack client")
		conntracker, err = conntrack.NewCiliumClient(log, conntrack.ClockSource(*ciliumClockSource))
	} else {
		log.Info("using netfilter conntrack client")
		conntracker, err = conntrack.NewNetfilterClient(log, *hostPid)
	}
	if err != nil {
		return err
	}

	var ip2dns dns.DNSCollector = &dns.Noop{}
	if *ebpfDNSTracerEnabled && ebpf.IsKernelBTFAvailable() {
		tracer := ebpf.NewTracer(log, ebpf.Config{
			QueueSize: *ebpfDNSTracerQueueSize,
		})
		ip2dns = dns.NewIP2DNS(tracer, log)
	}

	cfg := collector.Config{
		ReadInterval:      *readInterval,
		CleanupInterval:   *cleanupInterval,
		NodeName:          os.Getenv("NODE_NAME"),
		ExcludeNamespaces: *excludeNamespaces,
		GroupPublicIPs:    *groupPublicIPs,
		SendTrafficDelta:  *sendTrafficDelta,
	}
	coll := collector.New(
		cfg,
		log,
		podsByNodeCache,
		conntracker,
		ip2dns,
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

	errc := make(chan error)
	go func() {
		if err := ip2dns.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			err = fmt.Errorf("starting ip2dns: %w", err)
			log.Error(err.Error())
			errc <- err
		}
		close(errc)
	}()

	go func() {
		if err := coll.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			err = fmt.Errorf("starting collector: %w", err)
			log.Error(err.Error())
			errc <- err
		}
		close(errc)
	}()

	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, context.Canceled) {
			err = fmt.Errorf("starting http server: %w", err)
			log.Error(err.Error())
			errc <- err
		}
		close(errc)
	}()

	return <-errc
}

// runInit runs once in init container.
func runInit(log logrus.FieldLogger) error {
	log.Infof("running init")
	defer log.Infof("init done")

	ciliumAvailable := conntrack.CiliumAvailable(*ctMode)
	if !ciliumAvailable {
		log.Info("init netfilter accounting")
		if err := conntrack.InitNetfilterAccounting(); err != nil {
			return fmt.Errorf("init nf conntrack: %w", err)
		}
	}

	if *initCgroupv2 {
		if err := ebpf.InitCgroupv2(log); err != nil {
			return fmt.Errorf("init cgroupv2: %w", err)
		}
	}

	return nil
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
	ciliumAvailable := conntrack.CiliumAvailable("")
	if ciliumAvailable {
		conntracker, err = conntrack.NewCiliumClient(log, conntrack.ClockSource(*ciliumClockSource))
	} else {
		conntracker, err = conntrack.NewNetfilterClient(log, *hostPid)
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
