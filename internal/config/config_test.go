package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const validYAML = `
endpoint: https://llm-gateway.example.com/v1
api_key: ""
models:
  - uni/deepseek-v4-flash-0731
  - uni/qwen3.6-27b
concurrency: 4
request_timeout: 120s
max_tokens_default: 4000
`

func TestParse_Valid(t *testing.T) {
	cfg, err := parse([]byte(validYAML))
	if err != nil {
		t.Fatalf("parse() error = %v", err)
	}
	if cfg.Endpoint != "https://llm-gateway.example.com/v1" {
		t.Errorf("Endpoint = %q", cfg.Endpoint)
	}
	if len(cfg.Models) != 2 {
		t.Errorf("Models = %v, want 2 entries", cfg.Models)
	}
	if cfg.Concurrency != 4 {
		t.Errorf("Concurrency = %d, want 4", cfg.Concurrency)
	}
	if cfg.RequestTimeout != 120*time.Second {
		t.Errorf("RequestTimeout = %v, want 120s", cfg.RequestTimeout)
	}
	if cfg.MaxTokensDefault != 4000 {
		t.Errorf("MaxTokensDefault = %d, want 4000", cfg.MaxTokensDefault)
	}
}

func TestParse_EnvOverridesAPIKey(t *testing.T) {
	t.Setenv(apiKeyEnvVar, "env-secret")
	cfg, err := parse([]byte(validYAML))
	if err != nil {
		t.Fatalf("parse() error = %v", err)
	}
	if cfg.APIKey != "env-secret" {
		t.Errorf("APIKey = %q, want env override", cfg.APIKey)
	}
}

func TestParse_FileAPIKeyUsedWhenNoEnv(t *testing.T) {
	// S10: guarantee no env var leakage from the operator's real shell -
	// without this, a machine with LLMTB_API_KEY already set would make
	// this test assert the wrong thing regardless of parse()'s behavior.
	t.Setenv(apiKeyEnvVar, "")

	yaml := `
endpoint: https://llm-gateway.example.com/v1
api_key: "file-key"
models:
  - uni/deepseek-v4-flash-0731
concurrency: 4
request_timeout: 120s
max_tokens_default: 4000
`
	cfg, err := parse([]byte(yaml))
	if err != nil {
		t.Fatalf("parse() error = %v", err)
	}
	if cfg.APIKey != "file-key" {
		t.Errorf("APIKey = %q, want file-key", cfg.APIKey)
	}
}

func TestParse_DefaultsAppliedWhenOmitted(t *testing.T) {
	yaml := `
endpoint: https://llm-gateway.example.com/v1
models:
  - uni/deepseek-v4-flash-0731
`
	cfg, err := parse([]byte(yaml))
	if err != nil {
		t.Fatalf("parse() error = %v", err)
	}
	if cfg.Concurrency != defaultConcurrency {
		t.Errorf("Concurrency = %d, want default %d", cfg.Concurrency, defaultConcurrency)
	}
	if cfg.RequestTimeout != defaultRequestTimeout {
		t.Errorf("RequestTimeout = %v, want default %v", cfg.RequestTimeout, defaultRequestTimeout)
	}
	if cfg.MaxTokensDefault != defaultMaxTokens {
		t.Errorf("MaxTokensDefault = %d, want default %d", cfg.MaxTokensDefault, defaultMaxTokens)
	}
}

func TestParse_DefaultsDoNotOverrideExplicitValues(t *testing.T) {
	cfg, err := parse([]byte(validYAML))
	if err != nil {
		t.Fatalf("parse() error = %v", err)
	}
	// validYAML sets concurrency=4, request_timeout=120s,
	// max_tokens_default=4000, all below the new defaults; an explicit
	// value, even a small one, must still win over the default floor
	// applied by applyDefaults.
	if cfg.Concurrency != 4 {
		t.Errorf("Concurrency = %d, want the explicit 4, not the default", cfg.Concurrency)
	}
	if cfg.RequestTimeout != 120*time.Second {
		t.Errorf("RequestTimeout = %v, want the explicit 120s, not the default", cfg.RequestTimeout)
	}
	if cfg.MaxTokensDefault != 4000 {
		t.Errorf("MaxTokensDefault = %d, want the explicit 4000, not the default", cfg.MaxTokensDefault)
	}
}

func TestParse_InvalidYAML(t *testing.T) {
	_, err := parse([]byte("not: [valid: yaml"))
	if err == nil {
		t.Fatal("parse() error = nil, want error for invalid YAML")
	}
}

func TestParse_InvalidDuration(t *testing.T) {
	yaml := `
endpoint: https://x
models: [m]
concurrency: 1
request_timeout: not-a-duration
max_tokens_default: 100
`
	_, err := parse([]byte(yaml))
	if err == nil {
		t.Fatal("parse() error = nil, want error for invalid duration")
	}
}

func TestValidate(t *testing.T) {
	base := Config{
		Endpoint:         "https://x",
		Models:           []string{"m"},
		Concurrency:      1,
		RequestTimeout:   time.Second,
		MaxTokensDefault: 100,
	}

	if err := base.Validate(); err != nil {
		t.Fatalf("Validate() on valid config error = %v", err)
	}

	tests := []struct {
		mutate func(c Config) Config
		name   string
	}{
		{name: "empty endpoint", mutate: func(c Config) Config { c.Endpoint = ""; return c }},
		{name: "no models", mutate: func(c Config) Config { c.Models = nil; return c }},
		{name: "empty model entry", mutate: func(c Config) Config { c.Models = []string{""}; return c }},
		{name: "zero concurrency", mutate: func(c Config) Config { c.Concurrency = 0; return c }},
		{name: "negative concurrency", mutate: func(c Config) Config { c.Concurrency = -1; return c }},
		{name: "zero timeout", mutate: func(c Config) Config { c.RequestTimeout = 0; return c }},
		{name: "zero max tokens", mutate: func(c Config) Config { c.MaxTokensDefault = 0; return c }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := tt.mutate(base)
			if err := c.Validate(); err == nil {
				t.Errorf("Validate() on %s: error = nil, want error", tt.name)
			}
		})
	}
}

func TestLoad_ReadsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(validYAML), 0o600); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.Endpoint == "" {
		t.Error("Load() returned empty Endpoint")
	}
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "does-not-exist.yaml"))
	if err == nil {
		t.Fatal("Load() error = nil, want error for missing file")
	}
}
