package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	DefaultAddr        = ":8080"
	DefaultBaseURL     = "https://api.frankfurter.dev"
	DefaultProvider    = "ecb"
	DefaultTransport   = "http"
	DefaultHTTPTimeout = 15 * time.Second
)

type Config struct {
	Addr         string
	BaseURL      string
	Provider     string
	Transport    string
	HTTPTimeout  time.Duration
	UserAgent    string
}

func FromEnv() (Config, error) {
	cfg := Config{
		Addr:        envOr("MARKET_MCP_ADDR", DefaultAddr),
		BaseURL:     strings.TrimRight(envOr("MARKET_MCP_BASE_URL", DefaultBaseURL), "/"),
		Provider:    strings.ToLower(envOr("MARKET_MCP_PROVIDER", DefaultProvider)),
		Transport:   strings.ToLower(envOr("MARKET_MCP_TRANSPORT", DefaultTransport)),
		HTTPTimeout: DefaultHTTPTimeout,
		UserAgent:   envOr("MARKET_MCP_USER_AGENT", "longshift-market-mcp/0.1"),
	}
	if raw := os.Getenv("MARKET_MCP_HTTP_TIMEOUT"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("MARKET_MCP_HTTP_TIMEOUT: %w", err)
		}
		cfg.HTTPTimeout = d
	}
	switch cfg.Transport {
	case "http", "stdio":
	default:
		return Config{}, fmt.Errorf("MARKET_MCP_TRANSPORT must be http or stdio, got %q", cfg.Transport)
	}
	if cfg.BaseURL == "" {
		return Config{}, fmt.Errorf("MARKET_MCP_BASE_URL is empty")
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
