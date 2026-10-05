package frankfurter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRatesUsesProviderPathAndQuery(t *testing.T) {
	var gotPath, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		_ = json.NewEncoder(w).Encode([]Rate{{
			Date: "2024-01-02", Base: "EUR", Quote: "USD", Rate: 1.0956,
		}})
	}))
	t.Cleanup(srv.Close)

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client(), DefaultProvider: "ecb"}
	got, err := c.Rates(context.Background(), RatesQuery{
		Base:   "eur",
		Quotes: []string{"usd", "gbp"},
		Date:   "2024-01-02",
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v2/providers/ecb/rates" {
		t.Fatalf("path = %q", gotPath)
	}
	if want := "base=EUR&date=2024-01-02&quotes=USD%2CGBP"; gotQuery != want {
		t.Fatalf("query = %q, want %q", gotQuery, want)
	}
	if len(got) != 1 || got[0].Rate != 1.0956 {
		t.Fatalf("rates = %#v", got)
	}
}

func TestRateBlendedWhenProviderAll(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewEncoder(w).Encode(Rate{Date: "2026-10-05", Base: "EUR", Quote: "USD", Rate: 1.12})
	}))
	t.Cleanup(srv.Close)

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client(), DefaultProvider: "ecb"}
	got, err := c.Rate(context.Background(), "eur", "usd", "", "all")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v2/rate/EUR/USD" {
		t.Fatalf("path = %q", gotPath)
	}
	if got.Rate != 1.12 {
		t.Fatalf("rate = %v", got.Rate)
	}
}

func TestConvertMultipliesAmount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Rate{Date: "2026-10-02", Base: "EUR", Quote: "USD", Rate: 1.1})
	}))
	t.Cleanup(srv.Close)

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client(), DefaultProvider: "ecb"}
	got, err := c.Convert(context.Background(), "EUR", "USD", 10, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Converted != 11 {
		t.Fatalf("converted = %v", got.Converted)
	}
	if got.Provider != "ecb" {
		t.Fatalf("provider = %q", got.Provider)
	}
}

func TestAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		_ = json.NewEncoder(w).Encode(APIError{Status: 422, Message: "invalid currency: ZZZ"})
	}))
	t.Cleanup(srv.Close)

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	_, err := c.Rate(context.Background(), "EUR", "ZZZ", "", "all")
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("err type %T: %v", err, err)
	}
	if apiErr.Status != 422 || apiErr.Message != "invalid currency: ZZZ" {
		t.Fatalf("api error = %#v", apiErr)
	}
}

func TestCurrenciesAndProviders(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/currencies", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("scope") != "all" {
			t.Errorf("scope = %q", r.URL.Query().Get("scope"))
		}
		_ = json.NewEncoder(w).Encode([]Currency{{ISOCode: "EUR", Name: "Euro"}})
	})
	mux.HandleFunc("/v2/providers", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode([]Provider{{Key: "ECB", Name: "European Central Bank"}})
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	c := &Client{BaseURL: srv.URL, HTTP: srv.Client()}
	curs, err := c.Currencies(context.Background(), "all")
	if err != nil || len(curs) != 1 || curs[0].ISOCode != "EUR" {
		t.Fatalf("currencies = %#v err=%v", curs, err)
	}
	provs, err := c.Providers(context.Background())
	if err != nil || len(provs) != 1 || provs[0].Key != "ECB" {
		t.Fatalf("providers = %#v err=%v", provs, err)
	}
}
