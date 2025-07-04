package sinks

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"time"

	"github.com/google/uuid"
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

	reqID := uuid.NewString()

	trace := &httptrace.ClientTrace{
		GotConn: func(connInfo httptrace.GotConnInfo) {
			fmt.Printf("http_sink_trace ts=%s req_id=%s method=GotConn: %+v remote=%s\n", time.Now().UTC().Format(time.RFC3339Nano), reqID, connInfo, connInfo.Conn.RemoteAddr().String())
		},
		DNSDone: func(dnsInfo httptrace.DNSDoneInfo) {
			fmt.Printf("http_sink_trace ts=%s req_id=%s method=DNSDone: %+v\n", time.Now().UTC().Format(time.RFC3339Nano), reqID, dnsInfo)
		},
		DNSStart: func(dnsInfo httptrace.DNSStartInfo) {
			fmt.Printf("http_sink_trace ts=%s req_id=%s method=DNSStart: %+v\n", time.Now().UTC().Format(time.RFC3339Nano), reqID, dnsInfo)
		},
		TLSHandshakeStart: func() {
			fmt.Printf("http_sink_trace ts=%s req_id=%s method=TLSHandshakeStart\n", time.Now().UTC().Format(time.RFC3339Nano), reqID)
		},
		TLSHandshakeDone: func(state tls.ConnectionState, err error) {
			fmt.Printf("http_sink_trace ts=%s req_id=%s method=TLSHandshakeDone err=%v\n", time.Now().UTC().Format(time.RFC3339Nano), reqID, err)
		},
	}
	ctx = httptrace.WithClientTrace(ctx, trace)

	req, err := http.NewRequestWithContext(ctx, s.cfg.Method, uri.String(), payloadBuf)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}
	req.Header = header
	req.Header.Add("X-Request-ID", reqID)

	s.log.Infof("pushing metrics, req_id=%s, items=%d, size_bytes=%d", reqID, len(batch.Items), payloadBuf.Len())

	var resp *http.Response
	backoff := wait.Backoff{
		Duration: 100 * time.Millisecond,
		Factor:   1.5,
		Jitter:   0.2,
		Steps:    3,
	}
	err = wait.ExponentialBackoffWithContext(ctx, backoff, func(ctx context.Context) (done bool, err error) {
		resp, err = s.httpClient.Do(req) //nolint:bodyclose
		if err != nil {
			s.log.Warnf("failed sending request, req_id=%s, %v", reqID, err)
			return false, fmt.Errorf("sending request, req_id=%s:%w", reqID, err)
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
		return fmt.Errorf("request error req_id=%s status_code=%d body=%s url=%s", reqID, resp.StatusCode, buf.String(), uri.String())
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
