package server

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ecociel/longshift-market-mcp/internal/config"
	"github.com/ecociel/longshift-market-mcp/internal/frankfurter"
)

const Version = "0.1.0"

type Market struct {
	Client *frankfurter.Client
}

func New(cfg config.Config, httpClient *http.Client) *mcp.Server {
	m := &Market{
		Client: &frankfurter.Client{
			BaseURL:         cfg.BaseURL,
			HTTP:            httpClient,
			UserAgent:       cfg.UserAgent,
			DefaultProvider: cfg.Provider,
		},
	}
	if m.Client.HTTP == nil {
		m.Client.HTTP = &http.Client{Timeout: cfg.HTTPTimeout}
	}

	s := mcp.NewServer(&mcp.Implementation{
		Name:    "longshift-market-mcp",
		Version: Version,
	}, &mcp.ServerOptions{
		Instructions: "Official and blended FX reference rates from Frankfurter (https://www.frankfurter.app/). Default provider is the ECB. No API key is used.",
	})
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_currencies",
		Description: "List currencies known to Frankfurter, including ISO code, name, and coverage dates.",
	}, m.listCurrencies)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_providers",
		Description: "List official rate providers (central banks and similar). Default rate tools use ECB.",
	}, m.listProviders)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_rates",
		Description: "Fetch latest, historical, or time-series FX rates. Defaults to ECB official reference rates.",
	}, m.getRates)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "get_rate",
		Description: "Fetch a single FX pair (base/quote). Defaults to ECB official reference rates.",
	}, m.getRate)
	mcp.AddTool(s, &mcp.Tool{
		Name:        "convert",
		Description: "Convert an amount from one currency to another using a Frankfurter rate. Defaults to ECB.",
	}, m.convert)
	return s
}

func HTTPHandler(s *mcp.Server) http.Handler {
	mcpHandler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return s
	}, nil)
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.Handle("/mcp", mcpHandler)
	mux.Handle("/mcp/", mcpHandler)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"name":    "longshift-market-mcp",
			"version": Version,
			"mcp":     "/mcp",
			"health":  "/healthz",
		})
	})
	return mux
}

type listCurrenciesInput struct {
	Scope string `json:"scope,omitempty" jsonschema:"Use 'all' to include legacy currencies. Omit for current currencies."`
}

type listProvidersInput struct{}

type getRatesInput struct {
	Base     string   `json:"base,omitempty" jsonschema:"ISO base currency, e.g. EUR. ECB rates are quoted against EUR."`
	Quotes   []string `json:"quotes,omitempty" jsonschema:"Optional list of quote currencies, e.g. USD, GBP."`
	Date     string   `json:"date,omitempty" jsonschema:"Single historical date YYYY-MM-DD. Omit for the latest available fixing."`
	From     string   `json:"from,omitempty" jsonschema:"Time-series start date YYYY-MM-DD."`
	To       string   `json:"to,omitempty" jsonschema:"Time-series end date YYYY-MM-DD."`
	Group    string   `json:"group,omitempty" jsonschema:"Optional downsample: week or month."`
	Provider string   `json:"provider,omitempty" jsonschema:"Provider key such as ecb. Use 'all' for Frankfurter's blended feed. Default is the server MARKET_MCP_PROVIDER (ecb)."`
}

type getRateInput struct {
	Base     string `json:"base" jsonschema:"ISO base currency, e.g. EUR"`
	Quote    string `json:"quote" jsonschema:"ISO quote currency, e.g. USD"`
	Date     string `json:"date,omitempty" jsonschema:"Optional historical date YYYY-MM-DD"`
	Provider string `json:"provider,omitempty" jsonschema:"Provider key such as ecb, or 'all' for blended rates"`
}

type convertInput struct {
	Amount   float64 `json:"amount" jsonschema:"Amount in the base currency"`
	Base     string  `json:"base" jsonschema:"ISO currency of the amount"`
	Quote    string  `json:"quote" jsonschema:"ISO currency to convert into"`
	Date     string  `json:"date,omitempty" jsonschema:"Optional historical date YYYY-MM-DD"`
	Provider string  `json:"provider,omitempty" jsonschema:"Provider key such as ecb, or 'all' for blended rates"`
}

func (m *Market) listCurrencies(ctx context.Context, _ *mcp.CallToolRequest, in listCurrenciesInput) (*mcp.CallToolResult, any, error) {
	out, err := m.Client.Currencies(ctx, in.Scope)
	return toolResult(out, err)
}

func (m *Market) listProviders(ctx context.Context, _ *mcp.CallToolRequest, _ listProvidersInput) (*mcp.CallToolResult, any, error) {
	out, err := m.Client.Providers(ctx)
	return toolResult(out, err)
}

func (m *Market) getRates(ctx context.Context, _ *mcp.CallToolRequest, in getRatesInput) (*mcp.CallToolResult, any, error) {
	out, err := m.Client.Rates(ctx, frankfurter.RatesQuery{
		Base:     in.Base,
		Quotes:   in.Quotes,
		Date:     in.Date,
		From:     in.From,
		To:       in.To,
		Group:    in.Group,
		Provider: in.Provider,
	})
	return toolResult(out, err)
}

func (m *Market) getRate(ctx context.Context, _ *mcp.CallToolRequest, in getRateInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Base) == "" || strings.TrimSpace(in.Quote) == "" {
		return toolError("base and quote are required")
	}
	out, err := m.Client.Rate(ctx, in.Base, in.Quote, in.Date, in.Provider)
	return toolResult(out, err)
}

func (m *Market) convert(ctx context.Context, _ *mcp.CallToolRequest, in convertInput) (*mcp.CallToolResult, any, error) {
	if strings.TrimSpace(in.Base) == "" || strings.TrimSpace(in.Quote) == "" {
		return toolError("base and quote are required")
	}
	out, err := m.Client.Convert(ctx, in.Base, in.Quote, in.Amount, in.Date, in.Provider)
	return toolResult(out, err)
}

func toolResult(out any, err error) (*mcp.CallToolResult, any, error) {
	if err != nil {
		return toolError(err.Error())
	}
	return nil, out, nil
}

func toolError(msg string) (*mcp.CallToolResult, any, error) {
	return &mcp.CallToolResult{
		IsError: true,
		Content: []mcp.Content{&mcp.TextContent{Text: msg}},
	}, nil, nil
}
