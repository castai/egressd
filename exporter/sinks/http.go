package sinks

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/sirupsen/logrus"
	"google.golang.org/protobuf/proto"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/castai/egressd/exporter/config"
	"github.com/castai/egressd/pb"
)

const (
	headerUserAgent       = "User-Agent"
	headerContentType     = "Content-Type"
	headerContentEncoding = "Content-Encoding"
)

func NewHTTPSink(log logrus.FieldLogger, sinkName string, cfg config.SinkHTTPConfig, binVersion string) Sink {
	return &HTTPSink{
		log: log.WithFields(map[string]interface{}{
			"sink_type": "http",
			"sink_name": sinkName,
		}),
		httpClient: newHTTPClient(),
		cfg:        cfg,
		binVersion: binVersion,
	}
}

type HTTPSink struct {
	log        logrus.FieldLogger
	httpClient *http.Client
	cfg        config.SinkHTTPConfig
	binVersion string
}

func (s *HTTPSink) Push(ctx context.Context, batch *pb.PodNetworkMetricBatch) error {
	uri, err := url.Parse(os.ExpandEnv(s.cfg.URL))
	if err != nil {
		return fmt.Errorf("invalid url: %w", err)
	}

	payloadBuf := new(bytes.Buffer)

	header := make(http.Header)
	for k, v := range s.cfg.Headers {
		header.Set(k, os.ExpandEnv(v))
	}
	if _, found := s.cfg.Headers[headerUserAgent]; !found {
		header.Set(headerUserAgent, "castai-egressd/"+s.binVersion)
	}

	switch s.cfg.Encoding {
	case config.EncodingProtobuf:
		header.Set(headerContentType, "application/protobuf")
		protoBytes, err := proto.Marshal(batch)
		if err != nil {
			return err
		}
		switch s.cfg.Compression {
		case config.CompressionGzip:
			header.Set(headerContentEncoding, "gzip")
			gzWriter := gzip.NewWriter(payloadBuf)
			if _, err := gzWriter.Write(protoBytes); err != nil {
				return err
			}
			if err := gzWriter.Close(); err != nil {
				return err
			}
		default:
			payloadBuf.Write(protoBytes)
		}
	default:
		return fmt.Errorf("encoding %q is not supported", s.cfg.Encoding)
	}

	timeout := s.cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, s.cfg.Method, uri.String(), payloadBuf)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header = header

	s.log.Infof("pushing metrics, items=%d, size_bytes=%d", len(batch.Items), payloadBuf.Len())

	var resp *http.Response
	backoff := wait.Backoff{
		Duration: 10 * time.Millisecond,
		Factor:   1.5,
		Jitter:   0.2,
		Steps:    3,
	}
	err = wait.ExponentialBackoffWithContext(ctx, backoff, func(ctx context.Context) (done bool, err error) {
		resp, err = s.httpClient.Do(req) //nolint:bodyclose
		if err != nil {
			s.log.Warnf("failed sending request: %v", err)
			return false, fmt.Errorf("sending request %w", err)
		}
		return true, nil
	})
	if err != nil {
		return err
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			s.log.Errorf("closing response body: %v", err)
		}
	}()

	if resp.StatusCode > 399 {
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(resp.Body); err != nil {
			s.log.Errorf("failed reading error response body: %v", err)
		}
		return fmt.Errorf("request error status_code=%d body=%s url=%s", resp.StatusCode, buf.String(), uri.String())
	}

	return nil
}

func newHTTPClient() *http.Client {
	dialer := &net.Dialer{
		Timeout: 5 * time.Second,
	}
	return &http.Client{
		Timeout: 2 * time.Minute,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			DialContext:           dialer.DialContext,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   5 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
}
