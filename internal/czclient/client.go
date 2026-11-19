// Package czclient calls the CloudZero REST API v2 cost and dimension endpoints.
package czclient

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"

	"github.com/rshade/finfocus-plugin-cloudzero/internal/config"
)

const (
	clientTimeout  = 30 * time.Second
	maxPages       = 100
	maxAttempts    = 3
	maxBodyBytes   = 8 << 20
	retryBaseDelay = 20 * time.Millisecond
	// rateLimitCooldown is how long a 429 stops further requests. CloudZero's
	// limit is 60 cost requests in a rolling 24 hours, so an immediate retry
	// would only spend budget on another 429.
	rateLimitCooldown = 5 * time.Minute
)

// CostQuery is one daily GET /billing/costs call, before cursor paging.
type CostQuery struct {
	Start    time.Time
	End      time.Time
	GroupBy  []string
	Filters  map[string][]string
	CostType string
}

// CostRow is one costs[] element. Dimensions are the extra keys whose names
// match the group_by dimension IDs.
type CostRow struct {
	UsageDate  time.Time
	Cost       float64
	Dimensions map[string]string
}

// Dimension is one element of GET /billing/dimensions.
type Dimension struct {
	ID   string
	Name string
}

// Client is a CloudZero HTTP client with a per-process request budget.
type Client struct {
	httpClient *http.Client
	logger     zerolog.Logger
	apiKey     string
	baseURL    string
	userAgent  string
	budget     int
	sleep      func(time.Duration)
	now        func() time.Time

	mu           sync.Mutex
	used         int
	limitedUntil time.Time
}

// New returns a client. An empty API key is stored and rejected at request time.
func New(cfg config.Config, version string, logger zerolog.Logger) *Client {
	return &Client{
		httpClient:   &http.Client{Timeout: clientTimeout},
		logger:       logger,
		apiKey:       cfg.APIKey,
		baseURL:      cfg.BaseURL,
		userAgent:    "finfocus-plugin-cloudzero/" + version,
		budget:       cfg.DailyBudget,
		sleep:        time.Sleep,
		now:          time.Now,
		mu:           sync.Mutex{},
		used:         0,
		limitedUntil: time.Time{},
	}
}

// RequestsUsed reports how many budgeted calls this process has made.
func (c *Client) RequestsUsed() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.used
}

// GetDimensions calls GET /billing/dimensions.
func (c *Client) GetDimensions(ctx context.Context) ([]Dimension, error) {
	body, code, err := c.get(ctx, "/billing/dimensions", nil, true)
	if err != nil {
		return nil, err
	}
	if code != http.StatusOK {
		return nil, newAPIError(statusKind(code), code)
	}
	return parseDimensions(body)
}

// GetCosts calls GET /billing/costs and follows cursors. Cursor pages do not
// spend budget. Every attempt at a first page spends budget, retries included,
// because CloudZero may count a failed call. A 410 restarts the query once.
// After a 429 on any request that spends budget, every such request fails with
// KindRateLimit for five minutes without being sent or spending budget. Cursor
// pages are still sent: CloudZero does not count them.
func (c *Client) GetCosts(ctx context.Context, q CostQuery) ([]CostRow, error) {
	rows, err := c.pages(ctx, q)
	var api *APIError
	if errors.As(err, &api) && api.Kind == KindExpired {
		return c.pages(ctx, q)
	}
	return rows, err
}

func (c *Client) pages(ctx context.Context, q CostQuery) ([]CostRow, error) {
	var (
		all    []CostRow
		cursor string
		seen   = map[string]struct{}{}
		charge = true
	)
	for range maxPages {
		body, code, err := c.get(ctx, "/billing/costs", queryValues(q, cursor), charge)
		charge = false
		if err != nil {
			return nil, err
		}
		if code != http.StatusOK {
			return nil, newAPIError(statusKind(code), code)
		}
		rows, next, hasNext, err := parseCosts(body)
		if err != nil {
			return nil, err
		}
		all = append(all, rows...)
		if !hasNext || next == "" {
			return all, nil
		}
		if _, dup := seen[next]; dup {
			return nil, newAPIError(KindMalformed, 0)
		}
		seen[next] = struct{}{}
		cursor = next
	}
	return nil, newAPIError(KindUnavailable, 0)
}

func (c *Client) get(ctx context.Context, path string, query url.Values, charge bool) ([]byte, int, error) {
	if c.apiKey == "" {
		return nil, 0, newAPIError(KindAuth, 0)
	}
	var last error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if charge {
			if err := c.admit(path); err != nil {
				return nil, 0, err
			}
		}
		body, code, retry, err := c.once(ctx, path, query)
		if code == http.StatusTooManyRequests {
			c.coolDown()
		}
		if err == nil && !retry {
			return body, code, nil
		}
		last = err
		if code >= http.StatusInternalServerError {
			last = newAPIError(KindUnavailable, code)
		}
		if attempt == maxAttempts || ctx.Err() != nil {
			break
		}
		c.sleep(retryBaseDelay * time.Duration(attempt))
	}
	if last == nil {
		last = newAPIError(KindUnavailable, 0)
	}
	return nil, 0, last
}

// admit lets a request that spends budget through: it fails during a rate-limit
// cooldown, and otherwise spends one unit of budget.
func (c *Client) admit(path string) error {
	if c.coolingDown() {
		c.logger.Debug().Str("path", path).Msg("cloudzero rate limit cooldown; request not sent")
		return newAPIError(KindRateLimit, 0)
	}
	return c.spend()
}

func (c *Client) coolDown() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.limitedUntil = c.now().Add(rateLimitCooldown)
}

func (c *Client) coolingDown() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now().Before(c.limitedUntil)
}

func (c *Client) spend() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.used >= c.budget {
		return newAPIError(KindBudget, 0)
	}
	c.used++
	return nil
}

func (c *Client) once(ctx context.Context, path string, query url.Values) ([]byte, int, bool, error) {
	endpoint := c.baseURL + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, false, newAPIError(KindBadRequest, 0)
	}
	req.Header.Set("Authorization", c.apiKey)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.logger.Warn().Err(withoutURL(err)).Str("path", path).Msg("cloudzero request failed")
		return nil, 0, true, newAPIError(KindUnavailable, 0)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
	if err != nil {
		return nil, resp.StatusCode, true, newAPIError(KindUnavailable, 0)
	}
	if resp.StatusCode >= http.StatusInternalServerError {
		c.logger.Warn().Int("status", resp.StatusCode).Str("path", path).Msg("cloudzero server error")
		return body, resp.StatusCode, true, nil
	}
	if resp.StatusCode != http.StatusOK {
		c.logger.Warn().Int("status", resp.StatusCode).Str("path", path).Msg("cloudzero request rejected")
	}
	return body, resp.StatusCode, false, nil
}

// withoutURL drops the request URL from a transport error. The URL carries the
// filters, which name accounts and resources.
func withoutURL(err error) error {
	if urlErr, ok := errors.AsType[*url.Error](err); ok {
		return urlErr.Err
	}
	return err
}

func queryValues(q CostQuery, cursor string) url.Values {
	v := url.Values{}
	v.Set("start_date", q.Start.UTC().Format(time.RFC3339))
	if !q.End.IsZero() {
		v.Set("end_date", q.End.UTC().Format(time.RFC3339))
	}
	v.Set("granularity", "daily")
	v.Set("cost_type", q.CostType)
	v.Set("align_to_granularity", "false")
	for _, dim := range q.GroupBy {
		v.Add("group_by", dim)
	}
	if len(q.Filters) > 0 {
		encoded, err := json.Marshal(q.Filters)
		if err == nil {
			v.Set("filters", string(encoded))
		}
	}
	if cursor != "" {
		v.Set("cursor", cursor)
	}
	return v
}

func parseCosts(body []byte) ([]CostRow, string, bool, error) {
	var payload struct {
		Costs      []map[string]any `json:"costs"`
		Pagination struct {
			Cursor struct {
				NextCursor *string `json:"next_cursor"`
				HasNext    bool    `json:"has_next"`
			} `json:"cursor"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, "", false, newAPIError(KindMalformed, 0)
	}
	rows := make([]CostRow, 0, len(payload.Costs))
	for _, raw := range payload.Costs {
		row, err := parseRow(raw)
		if err != nil {
			return nil, "", false, err
		}
		rows = append(rows, row)
	}
	next := ""
	if payload.Pagination.Cursor.NextCursor != nil {
		next = *payload.Pagination.Cursor.NextCursor
	}
	return rows, next, payload.Pagination.Cursor.HasNext, nil
}

func parseRow(raw map[string]any) (CostRow, error) {
	stamp, _ := raw["usage_date"].(string)
	when, err := time.Parse(time.RFC3339, stamp)
	if err != nil {
		return CostRow{}, newAPIError(KindMalformed, 0)
	}
	cost, err := floatValue(raw["cost"])
	if err != nil {
		return CostRow{}, newAPIError(KindMalformed, 0)
	}
	dims := make(map[string]string)
	for key, value := range raw {
		if key == "usage_date" || key == "cost" || key == "projected_row_count" {
			continue
		}
		text, ok := value.(string)
		if ok {
			dims[key] = text
		}
	}
	return CostRow{UsageDate: when.UTC(), Cost: cost, Dimensions: dims}, nil
}

func floatValue(v any) (float64, error) {
	switch n := v.(type) {
	case float64:
		return n, nil
	case json.Number:
		return n.Float64()
	case string:
		return strconv.ParseFloat(strings.TrimSpace(n), 64)
	default:
		return 0, errors.New("cost is not a number")
	}
}

func parseDimensions(body []byte) ([]Dimension, error) {
	var payload struct {
		Dimensions []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"dimensions"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, newAPIError(KindMalformed, 0)
	}
	out := make([]Dimension, 0, len(payload.Dimensions))
	for _, dim := range payload.Dimensions {
		out = append(out, Dimension{ID: dim.ID, Name: dim.Name})
	}
	return out, nil
}
