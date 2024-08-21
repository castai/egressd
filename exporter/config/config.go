package config

import (
	"errors"
	"os"
	"time"

	"github.com/kelseyhightower/envconfig"
	"gopkg.in/yaml.v3"
)

type Config struct {
	PodIP          string          `envconfig:"POD_IP" yaml:"podIP"`
	PodNamespace   string          `envconfig:"POD_NAMESPACE" yaml:"podNamespace"`
	ExportInterval time.Duration   `envconfig:"EXPORT_INTERVAL" yaml:"exportInterval"`
	Sinks          map[string]Sink `yaml:"sinks"`
}

type SinkType string

const (
	SinkTypeHTTP            SinkType = "http"
	SinkTypePromRemoteWrite SinkType = "prom_remote_write"
)

type Sink struct {
	HTTPConfig            *SinkHTTPConfig            `yaml:"http,omitempty"`
	PromRemoteWriteConfig *SinkPromRemoteWriteConfig `yaml:"prom_remote_write,omitempty"`
}

type Compression string

const (
	CompressionGzip Compression = "gzip"
)

type Encoding string

const (
	EncodingProtobuf Encoding = "protobuf"
)

type SinkHTTPConfig struct {
	URL         string            `yaml:"url"`
	Method      string            `yaml:"method"`
	Compression Compression       `yaml:"compression"`
	Encoding    Encoding          `yaml:"encoding"`
	Headers     map[string]string `yaml:"headers"`
	Timeout     time.Duration     `yaml:"timeout"`
}

type SinkPromRemoteWriteConfig struct {
	URL                     string            `yaml:"url"`
	Headers                 map[string]string `yaml:"headers"`
	Labels                  map[string]string `yaml:"labels"`
	SendReceivedBytesMetric bool              `yaml:"sendReceivedBytesMetric"`
}

func Load(configPath string) (Config, error) {
	var cfg Config
	// Load config from yaml file if specified.
	if configPath != "" {
		configBytes, err := os.ReadFile(configPath)
		if err != nil {
			return Config{}, err
		}
		if err := yaml.Unmarshal(configBytes, &cfg); err != nil {
			return Config{}, err
		}
	}
	// Override with evn variables (if any).
	if err := envconfig.Process("", &cfg); err != nil {
		return Config{}, err
	}

	if cfg.ExportInterval == 0 {
		cfg.ExportInterval = 60 * time.Second
	}

	if len(cfg.Sinks) == 0 {
		return Config{}, errors.New("at least one sink config is required")
	}

	return cfg, nil
}
