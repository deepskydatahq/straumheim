// Package config handles YAML configuration loading with env var substitution.
package config

import (
	"os"
	"regexp"
	"time"

	"gopkg.in/yaml.v3"
)

var envVarRe = regexp.MustCompile(`\$\{([^}]+)\}`)

// Config is the top-level configuration struct.
type Config struct {
	Server ServerConfig          `yaml:"server"`
	Inputs map[string]InputConfig `yaml:"inputs"`
	Buffer BufferConfig           `yaml:"buffer"`
	Sinks  []SinkConfig           `yaml:"sinks"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

// InputConfig holds settings for an input endpoint.
type InputConfig struct {
	Enabled bool   `yaml:"enabled"`
	Path    string `yaml:"path"`
}

// BufferConfig holds settings for the event buffer.
type BufferConfig struct {
	Type          string        `yaml:"type"`
	Capacity      int           `yaml:"capacity"`
	FlushInterval time.Duration `yaml:"flush_interval"`
	FlushCount    int           `yaml:"flush_count"`
}

// SinkConfig holds settings for an output sink.
type SinkConfig struct {
	Name          string        `yaml:"name"`
	Type          string        `yaml:"type"`
	Mode          string        `yaml:"mode"`
	DSN           string        `yaml:"dsn"`
	Table         string        `yaml:"table"`
	BatchSize     int           `yaml:"batch_size"`
	FlushInterval time.Duration `yaml:"flush_interval"`
	AutoSchema    bool          `yaml:"auto_schema"`
}

// LoadConfig reads a YAML file, substitutes ${ENV_VAR} references, and
// returns a Config with sensible defaults applied.
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	expanded := substituteEnvVars(string(data))

	cfg := &Config{}
	if err := yaml.Unmarshal([]byte(expanded), cfg); err != nil {
		return nil, err
	}

	applyDefaults(cfg)
	return cfg, nil
}

// substituteEnvVars replaces ${VAR} patterns with os.Getenv values.
func substituteEnvVars(input string) string {
	return envVarRe.ReplaceAllStringFunc(input, func(match string) string {
		varName := envVarRe.FindStringSubmatch(match)[1]
		return os.Getenv(varName)
	})
}

func applyDefaults(cfg *Config) {
	if cfg.Server.Port == 0 {
		cfg.Server.Port = 8080
	}
	if cfg.Buffer.Type == "" {
		cfg.Buffer.Type = "memory"
	}
	if cfg.Buffer.Capacity == 0 {
		cfg.Buffer.Capacity = 10000
	}
	if cfg.Buffer.FlushInterval == 0 {
		cfg.Buffer.FlushInterval = 5 * time.Second
	}
	if cfg.Buffer.FlushCount == 0 {
		cfg.Buffer.FlushCount = 500
	}
}
