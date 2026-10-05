package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/ecociel/longshift-market-mcp/internal/config"
	"github.com/ecociel/longshift-market-mcp/internal/frankfurter"
)

func TestHealthzAndIndex(t *testing.T) {
	h := HTTPHandler(New(config.Config{BaseURL: "http://127.0.0.1:1", Provider: "ecb"}, http.DefaultClient))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || rr.Body.String() != "ok\n" {
		t.Fatalf("healthz: %d %q", rr.Code, rr.Body.String())
	}

	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("index status %d", rr.Code)
	}
	var info map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info["mcp"] != "/mcp" {
		t.Fatalf("index = %#v", info)
	}
}

func TestMCPToolsAgainstUpstream(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/currencies", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]frankfurter.Currency{{ISOCode: "EUR", Name: "Euro"}})
	})
	mux.HandleFunc("/v2/providers", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]frankfurter.Provider{{Key: "ECB", Name: "European Central Bank"}})
	})
	mux.HandleFunc("/v2/providers/ecb/rate/EUR/USD", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(frankfurter.Rate{Date: "2026-10-02", Base: "EUR", Quote: "USD", Rate: 1.1})
	})
	mux.HandleFunc("/v2/providers/ecb/rates", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]frankfurter.Rate{{Date: "2026-10-02", Base: "EUR", Quote: "USD", Rate: 1.1}})
	})
	upstream := httptest.NewServer(mux)
	t.Cleanup(upstream.Close)

	srv := New(config.Config{
		BaseURL:  upstream.URL,
		Provider: "ecb",
	}, upstream.Client())

	ctx := context.Background()
	session := connect(t, ctx, srv)

	tools, err := session.ListTools(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]bool{"list_currencies": false, "list_providers": false, "get_rates": false, "get_rate": false, "convert": false}
	for _, tool := range tools.Tools {
		if _, ok := want[tool.Name]; ok {
			want[tool.Name] = true
		}
	}
	for name, seen := range want {
		if !seen {
			t.Fatalf("missing tool %s", name)
		}
	}

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "convert",
		Arguments: map[string]any{"amount": 10.0, "base": "EUR", "quote": "USD"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("convert error: %#v", res)
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var conv frankfurter.Conversion
	if err := json.Unmarshal(raw, &conv); err != nil {
		t.Fatalf("structured %s: %v", raw, err)
	}
	if conv.Converted != 11 {
		t.Fatalf("converted = %#v", conv)
	}

	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_rate",
		Arguments: map[string]any{"base": "EUR", "quote": "USD"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("get_rate error: %#v", res)
	}

	res, err = session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "get_rates",
		Arguments: map[string]any{"base": "EUR", "quotes": []string{"USD"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("get_rates error: %#v", res)
	}

	res, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "list_currencies", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("list_currencies error: %#v", res)
	}

	res, err = session.CallTool(ctx, &mcp.CallToolParams{Name: "list_providers", Arguments: map[string]any{}})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsError {
		t.Fatalf("list_providers error: %#v", res)
	}
}

func TestConvertToolSurfacesUpstreamError(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(frankfurter.APIError{Status: 422, Message: "invalid currency: ZZZ"})
	}))
	t.Cleanup(upstream.Close)

	srv := New(config.Config{BaseURL: upstream.URL, Provider: "ecb"}, upstream.Client())
	ctx := context.Background()
	session := connect(t, ctx, srv)

	res, err := session.CallTool(ctx, &mcp.CallToolParams{
		Name:      "convert",
		Arguments: map[string]any{"amount": 1.0, "base": "EUR", "quote": "ZZZ"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsError {
		t.Fatalf("expected tool error, got %#v", res)
	}
}

func connect(t *testing.T, ctx context.Context, srv *mcp.Server) *mcp.ClientSession {
	t.Helper()
	clientTransport, serverTransport := mcp.NewInMemoryTransports()
	go func() {
		_ = srv.Run(ctx, serverTransport)
	}()
	client := mcp.NewClient(&mcp.Implementation{Name: "test-client", Version: "0.0.1"}, nil)
	session, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}
