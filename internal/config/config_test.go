package config

import (
	"testing"
	"time"
)

func TestFromEnvDefaults(t *testing.T) {
	t.Setenv("MARKET_MCP_ADDR", "")
	t.Setenv("MARKET_MCP_BASE_URL", "")
	t.Setenv("MARKET_MCP_PROVIDER", "")
	t.Setenv("MARKET_MCP_TRANSPORT", "")
	t.Setenv("MARKET_MCP_HTTP_TIMEOUT", "")
	t.Setenv("MARKET_MCP_USER_AGENT", "")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != DefaultAddr || cfg.BaseURL != DefaultBaseURL || cfg.Provider != DefaultProvider {
		t.Fatalf("%+v", cfg)
	}
	if cfg.Transport != "http" || cfg.HTTPTimeout != DefaultHTTPTimeout {
		t.Fatalf("%+v", cfg)
	}
}

func TestFromEnvOverridesAndValidation(t *testing.T) {
	t.Setenv("MARKET_MCP_ADDR", "127.0.0.1:9")
	t.Setenv("MARKET_MCP_BASE_URL", "https://example.test/")
	t.Setenv("MARKET_MCP_PROVIDER", "ALL")
	t.Setenv("MARKET_MCP_TRANSPORT", "stdio")
	t.Setenv("MARKET_MCP_HTTP_TIMEOUT", "2s")

	cfg, err := FromEnv()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Addr != "127.0.0.1:9" || cfg.BaseURL != "https://example.test" {
		t.Fatalf("%+v", cfg)
	}
	if cfg.Provider != "all" || cfg.Transport != "stdio" || cfg.HTTPTimeout != 2*time.Second {
		t.Fatalf("%+v", cfg)
	}

	t.Setenv("MARKET_MCP_TRANSPORT", "unix")
	if _, err := FromEnv(); err == nil {
		t.Fatal("expected transport error")
	}
}
