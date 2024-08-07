package sinks

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"github.com/castai/egressd/exporter/config"
	"github.com/castai/egressd/pb"
)

func TestHTTPSink(t *testing.T) {
	r := require.New(t)
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	reqBytesCh := make(chan []byte, 1)
	reqHeaders := make(chan http.Header, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		reqBytes, err := io.ReadAll(req.Body)
		reqBytesCh <- reqBytes
		reqHeaders <- req.Header
		r.NoError(err) //nolint:testifylint
	}))
	defer srv.Close()

	cfg := config.SinkHTTPConfig{
		URL:         srv.URL,
		Method:      "POST",
		Compression: config.CompressionGzip,
		Encoding:    config.EncodingProtobuf,
		Headers: map[string]string{
			"Custom-Header": "1",
		},
		Timeout: 10 * time.Second,
	}
	sink := NewHTTPSink(log, "http-test", cfg, "0.0.0")
	expectedBatch := &pb.PodNetworkMetricBatch{
		Items: []*pb.PodNetworkMetric{
			{
				SrcIp:        "10.10.2.14",
				SrcPod:       "p1",
				SrcNamespace: "team1",
				SrcNode:      "n1",
				SrcZone:      "us-east-1a",
				DstIp:        "10.10.2.15",
				DstPod:       "p2",
				DstNamespace: "team2",
				DstNode:      "n1",
				DstZone:      "us-east-1a",
				TxBytes:      35,
				TxPackets:    3,
				RxBytes:      30,
				RxPackets:    1,
				Proto:        6,
			},
		},
	}
	r.NoError(sink.Push(ctx, expectedBatch))
	reqBytes := <-reqBytesCh
	r.NotEmpty(reqBytes)

	gzReader, err := gzip.NewReader(bytes.NewBuffer(reqBytes))
	r.NoError(err)
	var pbBuf bytes.Buffer
	_, err = io.Copy(&pbBuf, gzReader) //nolint:gosec
	r.NoError(err)
	r.NoError(gzReader.Close())

	var actualBatch pb.PodNetworkMetricBatch
	r.NoError(proto.Unmarshal(pbBuf.Bytes(), &actualBatch))
	r.Len(actualBatch.Items, 1)
	r.Equal(expectedBatch.String(), actualBatch.String())

	headers := <-reqHeaders
	r.Equal("castai-egressd/0.0.0", headers[headerUserAgent][0])
	r.Equal("1", headers["Custom-Header"][0])
}
