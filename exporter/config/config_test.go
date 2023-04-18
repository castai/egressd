package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestConfig(t *testing.T) {
	t.Run("load config from file", func(t *testing.T) {
		r := require.New(t)
		expectedCfg := newTestConfig()
		expectedConfigBytes := []byte(`
podIP: 10.10.1.15
podNamespace: egressd
log:
  level: debug
exportInterval: 1m0s
sinks:
  castai:
    http:
      url: https://api.cast.ai
      method: POST
      compression: gzip
      encoding: protobuf
      headers:
        X-Api-Key: ${API_KEY_ENV_VAR}
      timeout: 30s
  prom:
    prom_remote_write:
      url: http://prometheus:8428/api/v1/write
      headers:
        X-Scope-OrgID: org-id
`)
		cfgFilePath := filepath.Join(t.TempDir(), "config.yaml")
		r.NoError(os.WriteFile(cfgFilePath, expectedConfigBytes, 0600))

		actualCfg, err := Load(cfgFilePath)
		r.NoError(err)

		r.Equal(expectedCfg, actualCfg)
	})

	t.Run("override config from env variables", func(t *testing.T) {
		r := require.New(t)
		expectedCfg := newTestConfig()
		expectedCfg.PodNamespace = "from-env"
		r.NoError(os.Setenv("POD_NAMESPACE", expectedCfg.PodNamespace))

		cfgBytes, err := yaml.Marshal(expectedCfg)
		r.NoError(err)
		cfgFilePath := filepath.Join(t.TempDir(), "config.yaml")
		r.NoError(os.WriteFile(cfgFilePath, cfgBytes, 0600))

		actualCfg, err := Load(cfgFilePath)
		r.NoError(err)
		r.Equal(expectedCfg, actualCfg)
	})
}

func newTestConfig() Config {
	return Config{
		PodIP:          "10.10.1.15",
		PodNamespace:   "egressd",
		ExportInterval: 60 * time.Second,
		Sinks: map[string]Sink{
			"castai": {
				HTTPConfig: &SinkHTTPConfig{
					URL:         "https://api.cast.ai",
					Method:      "POST",
					Compression: "gzip",
					Encoding:    "protobuf",
					Headers: map[string]string{
						"X-Api-Key": "${API_KEY_ENV_VAR}",
					},
					Timeout: 30 * time.Second,
				},
			},
			"prom": {
				PromRemoteWriteConfig: &SinkPromRemoteWriteConfig{
					URL: "http://prometheus:8428/api/v1/write",
					Headers: map[string]string{
						"X-Scope-OrgID": "org-id",
					},
				},
			},
		},
	}
}
