// Package config loads and validates the llm-testbench YAML configuration.
package config

import (
	"fmt"
	"net/url"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// apiKeyEnvVar names the environment variable that overrides Config.APIKey.
//
//nolint:gosec // this is an env var *name*, not a credential value
const apiKeyEnvVar = "LLMTB_API_KEY" // #nosec G101 -- this is an env var *name*, not a credential value

// Defaults applied when the corresponding YAML key is omitted (or explicit
// zero). max_tokens_default was raised from 4000 to 12000, then to 40000
// (2026-08-30: at 25000 the heaviest thinking-model tests still exhausted
// the whole budget on reasoning and scored 0 with an empty answer, e.g.
// ts-eslint-flat-config on qwen3.8-flash-next), after a live run
// showed reasoning models spend thousands of completion tokens on
// reasoning_content before writing an answer to content: a small
// per-test/default budget truncates the answer (finish_reason=length)
// before it is ever written, scoring a correct model as 0.
const (
	defaultConcurrency    = 8
	defaultRequestTimeout = 300 * time.Second
	defaultMaxTokens      = 40000
	defaultMaxRetries     = 2
)

// Config is the top-level configuration loaded from config.yaml.
type Config struct {
	Seed             *int                     `yaml:"seed"`
	ModelTimeouts    map[string]time.Duration `yaml:"-"`
	ModelConcurrency map[string]int           `yaml:"model_concurrency"`
	Endpoint         string                   `yaml:"endpoint"`
	APIKey           string                   `yaml:"api_key"`
	Models           []string                 `yaml:"models"`
	Concurrency      int                      `yaml:"concurrency"`
	RequestTimeout   time.Duration            `yaml:"request_timeout"`
	MaxTokensDefault int                      `yaml:"max_tokens_default"`
	MaxRetries       int                      `yaml:"max_retries"`
	IdleTimeout      time.Duration            `yaml:"-"`
	RetryTimeouts    bool                     `yaml:"retry_timeouts"`
	Stream           bool                     `yaml:"stream"`
}

// rawConfig mirrors Config but with RequestTimeout as a string, since
// yaml.v3 does not natively decode Go duration strings.
type rawConfig struct {
	MaxRetries       *int              `yaml:"max_retries"`
	Seed             *int              `yaml:"seed"`
	RetryTimeouts    *bool             `yaml:"retry_timeouts"`
	ModelTimeouts    map[string]string `yaml:"model_timeouts"`
	ModelConcurrency map[string]int    `yaml:"model_concurrency"`
	APIKey           string            `yaml:"api_key"`
	Endpoint         string            `yaml:"endpoint"`
	RequestTimeout   string            `yaml:"request_timeout"`
	IdleTimeout      string            `yaml:"idle_timeout"`
	Models           []string          `yaml:"models"`
	Concurrency      int               `yaml:"concurrency"`
	MaxTokensDefault int               `yaml:"max_tokens_default"`
	Stream           bool              `yaml:"stream"`
}

// Load reads and parses the YAML config at path, applies the LLMTB_API_KEY
// environment override (which always wins over the file value), and
// validates the result.
func Load(path string) (Config, error) {
	// #nosec G304 -- path is the operator's own --config flag value (a
	// local CLI tool reading its own config file), not externally supplied
	// or network input.
	data, err := os.ReadFile(path) //nolint:gosec // path is the operator's own --config flag value, not external input
	if err != nil {
		return Config{}, fmt.Errorf("config: read %s: %w", path, err)
	}
	return parse(data)
}

// parse decodes YAML bytes into a validated Config. Split out from Load for
// direct, file-free unit testing.
func parse(data []byte) (Config, error) {
	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return Config{}, fmt.Errorf("config: parse yaml: %w", err)
	}

	cfg := Config{
		Endpoint:         raw.Endpoint,
		APIKey:           raw.APIKey,
		Models:           raw.Models,
		Concurrency:      raw.Concurrency,
		MaxTokensDefault: raw.MaxTokensDefault,
		MaxRetries:       defaultMaxRetries,
		Seed:             raw.Seed,
	}
	if raw.MaxRetries != nil {
		cfg.MaxRetries = *raw.MaxRetries
	}
	cfg.RetryTimeouts = raw.RetryTimeouts == nil || *raw.RetryTimeouts
	cfg.ModelConcurrency = raw.ModelConcurrency
	cfg.Stream = raw.Stream
	if raw.IdleTimeout != "" {
		d, err := time.ParseDuration(raw.IdleTimeout)
		if err != nil {
			return Config{}, fmt.Errorf("config: parse idle_timeout %q: %w", raw.IdleTimeout, err)
		}
		cfg.IdleTimeout = d
	}
	if len(raw.ModelTimeouts) > 0 {
		cfg.ModelTimeouts = make(map[string]time.Duration, len(raw.ModelTimeouts))
		for m, v := range raw.ModelTimeouts {
			d, err := time.ParseDuration(v)
			if err != nil {
				return Config{}, fmt.Errorf("config: parse model_timeouts[%q] %q: %w", m, v, err)
			}
			cfg.ModelTimeouts[m] = d
		}
	}

	if raw.RequestTimeout != "" {
		d, err := time.ParseDuration(raw.RequestTimeout)
		if err != nil {
			return Config{}, fmt.Errorf("config: parse request_timeout %q: %w", raw.RequestTimeout, err)
		}
		cfg.RequestTimeout = d
	}

	applyDefaults(&cfg)

	if envKey := os.Getenv(apiKeyEnvVar); envKey != "" {
		cfg.APIKey = envKey
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// applyDefaults fills in Concurrency, RequestTimeout, and MaxTokensDefault
// when the YAML omitted them (or gave an explicit zero), so a minimal
// config.yaml still runs with sane, generous-enough values.
func applyDefaults(cfg *Config) {
	if cfg.Concurrency <= 0 {
		cfg.Concurrency = defaultConcurrency
	}
	if cfg.RequestTimeout <= 0 {
		cfg.RequestTimeout = defaultRequestTimeout
	}
	if cfg.MaxTokensDefault <= 0 {
		cfg.MaxTokensDefault = defaultMaxTokens
	}
}

// Validate checks that required fields are present and sane.
func (c Config) Validate() error {
	if c.Endpoint == "" {
		return fmt.Errorf("config: endpoint must not be empty")
	}
	if len(c.Models) == 0 {
		return fmt.Errorf("config: models must list at least one model")
	}
	u, err := url.Parse(c.Endpoint)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("config: endpoint %q must be an http(s) URL", c.Endpoint)
	}
	seen := make(map[string]bool, len(c.Models))
	for i, m := range c.Models {
		if m == "" {
			return fmt.Errorf("config: models[%d] must not be empty", i)
		}
		if seen[m] {
			return fmt.Errorf("config: models[%d] duplicates %q", i, m)
		}
		seen[m] = true
	}
	for m, d := range c.ModelTimeouts {
		if d <= 0 {
			return fmt.Errorf("config: model_timeouts[%q] must be > 0, got %s", m, d)
		}
	}
	for m, n := range c.ModelConcurrency {
		if n <= 0 {
			return fmt.Errorf("config: model_concurrency[%q] must be > 0, got %d", m, n)
		}
	}
	if c.IdleTimeout < 0 {
		return fmt.Errorf("config: idle_timeout must be >= 0, got %s", c.IdleTimeout)
	}
	if c.MaxRetries < 0 {
		return fmt.Errorf("config: max_retries must be >= 0, got %d", c.MaxRetries)
	}
	if c.Concurrency <= 0 {
		return fmt.Errorf("config: concurrency must be > 0, got %d", c.Concurrency)
	}
	if c.RequestTimeout <= 0 {
		return fmt.Errorf("config: request_timeout must be > 0, got %s", c.RequestTimeout)
	}
	if c.MaxTokensDefault <= 0 {
		return fmt.Errorf("config: max_tokens_default must be > 0, got %d", c.MaxTokensDefault)
	}
	return nil
}
