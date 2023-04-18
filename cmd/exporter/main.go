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
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/flowcontrol"

	"github.com/castai/egressd/exporter"
	"github.com/castai/egressd/exporter/config"
	"github.com/castai/egressd/exporter/sinks"
	"github.com/castai/egressd/kube"
)

var (
	kubeconfig     = flag.String("kubeconfig", "", "")
	logLevel       = flag.String("log-level", logrus.InfoLevel.String(), "Log level")
	httpListenPort = flag.Int("http-listen-port", 6060, "HTTP server listen port")
	configPath     = flag.String("config-path", "/etc/egressd/config/config.yaml", "Path to exporter config path")
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

	if err := run(log); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatal(err)
	}
}

func run(log logrus.FieldLogger) error {
	mux := http.NewServeMux()
	addPprofHandlers(mux)
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", healthHandler)

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

		ctx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Errorf("http server shutdown: %v", err)
		}

		cancel()
	}()

	restconfig, err := retrieveKubeConfig(log, *kubeconfig)
	if err != nil {
		return err
	}
	restconfig.RateLimiter = flowcontrol.NewTokenBucketRateLimiter(float32(25), 100)
	clientset, err := kubernetes.NewForConfig(restconfig)
	if err != nil {
		return err
	}

	informersFactory := informers.NewSharedInformerFactoryWithOptions(clientset, 30*time.Second)
	podsInformer := informersFactory.Core().V1().Pods().Informer()
	nodesInformer := informersFactory.Core().V1().Nodes().Informer()
	podsByNodeCache := kube.NewPodsByNodeCache(podsInformer)
	podByIPCache := kube.NewPodByIPCache(podsInformer)
	nodeByNameCache := kube.NewNodeByNameCache(nodesInformer)
	nodeByIPCache := kube.NewNodeByIPCache(nodesInformer)
	informersFactory.Start(wait.NeverStop)
	informersFactory.WaitForCacheSync(wait.NeverStop)

	kw := &kubeWatcher{
		podsByNode: podsByNodeCache,
		podByIP:    podByIPCache,
		nodeByName: nodeByNameCache,
		nodeByIP:   nodeByIPCache,
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		return err
	}

	var sinksList []sinks.Sink
	for name, s := range cfg.Sinks {
		if s.HTTPConfig != nil {
			sinksList = append(sinksList, sinks.NewHTTPSink(
				log,
				name,
				*s.HTTPConfig,
				Version,
			))
		} else if s.PromRemoteWriteConfig != nil {
			sinksList = append(sinksList, sinks.NewPromRemoteWriteSink(
				log, name, *s.PromRemoteWriteConfig,
			))
		}
	}

	if len(sinksList) == 0 {
		return errors.New("not sinks configured")
	}

	ex := exporter.New(log, cfg, kw, clientset, sinksList)
	go func() {
		if err := ex.Start(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Errorf("exporter start: %v", err)
			return
		}
	}()

	log.Infof("running egressd-exporter, version=%s, commit=%s, ref=%s, export_interval=%s, sinks=%d", Version, GitCommit, GitRef, cfg.ExportInterval, len(sinksList))
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

type kubeWatcher struct {
	podsByNode *kube.PodsByNodeCache
	podByIP    *kube.PodByIPCache
	nodeByName *kube.NodeByNameCache
	nodeByIP   *kube.NodeByIPCache
}

func (k *kubeWatcher) GetPodsByNode(nodeName string) ([]*v1.Pod, error) {
	return k.podsByNode.Get(nodeName)
}

func (k *kubeWatcher) GetPodByIP(ip string) (*v1.Pod, error) {
	return k.podByIP.Get(ip)
}

func (k *kubeWatcher) GetNodeByName(name string) (*v1.Node, error) {
	return k.nodeByName.Get(name)
}

func (k *kubeWatcher) GetNodeByIP(ip string) (*v1.Node, error) {
	return k.nodeByIP.Get(ip)
}
