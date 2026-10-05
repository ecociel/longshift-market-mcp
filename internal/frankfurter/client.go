package frankfurter

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

type Client struct {
	BaseURL    string
	HTTP       *http.Client
	UserAgent  string
	DefaultProvider string
}

type APIError struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

func (e *APIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("frankfurter: HTTP %d", e.Status)
	}
	return fmt.Sprintf("frankfurter: HTTP %d: %s", e.Status, e.Message)
}

type Rate struct {
	Date  string  `json:"date"`
	Base  string  `json:"base"`
	Quote string  `json:"quote"`
	Rate  float64 `json:"rate"`
}

type Currency struct {
	ISOCode    string `json:"iso_code"`
	ISONumeric string `json:"iso_numeric"`
	Name       string `json:"name"`
	Symbol     string `json:"symbol"`
	StartDate  string `json:"start_date"`
	EndDate    string `json:"end_date"`
}

type Provider struct {
	Key           string   `json:"key"`
	Name          string   `json:"name"`
	CountryCode   string   `json:"country_code"`
	RateType      string   `json:"rate_type"`
	PivotCurrency string   `json:"pivot_currency"`
	DataURL       string   `json:"data_url"`
	TermsURL      *string  `json:"terms_url"`
	StartDate     string   `json:"start_date"`
	EndDate       string   `json:"end_date"`
	Currencies    []string `json:"currencies,omitempty"`
}

type RatesQuery struct {
	Base      string
	Quotes    []string
	Date      string
	From      string
	To        string
	Group     string
	Provider  string
}

type Conversion struct {
	Date      string  `json:"date"`
	Base      string  `json:"base"`
	Quote     string  `json:"quote"`
	Rate      float64 `json:"rate"`
	Amount    float64 `json:"amount"`
	Converted float64 `json:"converted"`
	Provider  string  `json:"provider,omitempty"`
}

func (c *Client) Currencies(ctx context.Context, scope string) ([]Currency, error) {
	q := url.Values{}
	if scope != "" {
		q.Set("scope", scope)
	}
	var out []Currency
	if err := c.get(ctx, "/v2/currencies", q, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) Providers(ctx context.Context) ([]Provider, error) {
	var out []Provider
	if err := c.get(ctx, "/v2/providers", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) Rates(ctx context.Context, query RatesQuery) ([]Rate, error) {
	q := url.Values{}
	if query.Base != "" {
		q.Set("base", strings.ToUpper(query.Base))
	}
	if len(query.Quotes) > 0 {
		q.Set("quotes", strings.ToUpper(strings.Join(query.Quotes, ",")))
	}
	if query.Date != "" {
		q.Set("date", query.Date)
	}
	if query.From != "" {
		q.Set("from", query.From)
	}
	if query.To != "" {
		q.Set("to", query.To)
	}
	if query.Group != "" {
		q.Set("group", query.Group)
	}
	path := "/v2/rates"
	if p := c.provider(query.Provider); p != "" {
		path = "/v2/providers/" + url.PathEscape(p) + "/rates"
	}
	var out []Rate
	if err := c.get(ctx, path, q, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *Client) Rate(ctx context.Context, base, quote, date, provider string) (*Rate, error) {
	base = strings.ToUpper(strings.TrimSpace(base))
	quote = strings.ToUpper(strings.TrimSpace(quote))
	if base == "" || quote == "" {
		return nil, fmt.Errorf("base and quote are required")
	}
	q := url.Values{}
	if date != "" {
		q.Set("date", date)
	}
	path := "/v2/rate/" + url.PathEscape(base) + "/" + url.PathEscape(quote)
	if p := c.provider(provider); p != "" {
		path = "/v2/providers/" + url.PathEscape(p) + "/rate/" + url.PathEscape(base) + "/" + url.PathEscape(quote)
	}
	var out Rate
	if err := c.get(ctx, path, q, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) Convert(ctx context.Context, base, quote string, amount float64, date, provider string) (*Conversion, error) {
	rate, err := c.Rate(ctx, base, quote, date, provider)
	if err != nil {
		return nil, err
	}
	return &Conversion{
		Date:      rate.Date,
		Base:      rate.Base,
		Quote:     rate.Quote,
		Rate:      rate.Rate,
		Amount:    amount,
		Converted: amount * rate.Rate,
		Provider:  c.provider(provider),
	}, nil
}

func (c *Client) provider(explicit string) string {
	if p := strings.ToLower(strings.TrimSpace(explicit)); p != "" {
		if p == "all" || p == "blended" {
			return ""
		}
		return p
	}
	return strings.ToLower(strings.TrimSpace(c.DefaultProvider))
}

func (c *Client) get(ctx context.Context, path string, query url.Values, dest any) error {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return fmt.Errorf("base URL: %w", err)
	}
	u.Path = strings.TrimRight(u.Path, "/") + path
	if query != nil {
		u.RawQuery = query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 400 {
		var apiErr APIError
		if json.Unmarshal(body, &apiErr) == nil && (apiErr.Message != "" || apiErr.Status != 0) {
			if apiErr.Status == 0 {
				apiErr.Status = resp.StatusCode
			}
			return &apiErr
		}
		return &APIError{Status: resp.StatusCode, Message: strings.TrimSpace(string(body))}
	}
	if err := json.Unmarshal(body, dest); err != nil {
		return fmt.Errorf("decode frankfurter response: %w", err)
	}
	return nil
}
