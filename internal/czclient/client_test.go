package czclient

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rshade/finfocus-plugin-cloudzero/internal/config"
)

func TestGetCostsPagesCursorAndHeaders(t *testing.T) {
	t.Parallel()
	const key = "cz-test-key-not-for-logs"
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		assert.Equal(t, key, r.Header.Get("Authorization"))
		assert.NotContains(t, r.Header.Get("Authorization"), "Bearer")
		assert.Contains(t, r.Header.Get("User-Agent"), "finfocus-plugin-cloudzero/vtest")
		assert.Equal(t, "/v2/billing/costs", r.URL.Path)
		assert.ElementsMatch(t, []string{"Account", "Service"}, r.URL.Query()["group_by"])
		var filters map[string][]string
		require.NoError(t, json.Unmarshal([]byte(r.URL.Query().Get("filters")), &filters))
		assert.Equal(t, []string{"AWS"}, filters["CloudProvider"])
		if r.URL.Query().Get("cursor") == "" {
			writeJSON(t, w, map[string]any{
				"costs": []map[string]any{{
					"usage_date": "2023-10-02T00:00:00+00:00",
					"cost":       1.5,
					"Account":    "123",
				}},
				"pagination": map[string]any{"cursor": map[string]any{"next_cursor": "page-2", "has_next": true}},
			})
			return
		}
		assert.Equal(t, "page-2", r.URL.Query().Get("cursor"))
		writeJSON(t, w, map[string]any{
			"costs": []map[string]any{{
				"usage_date": "2023-10-03T00:00:00Z",
				"cost":       2,
			}},
			"pagination": map[string]any{"cursor": map[string]any{"next_cursor": nil, "has_next": false}},
		})
	}))
	t.Cleanup(srv.Close)

	client := testClient(t, srv.URL+"/v2", key, 10)
	rows, err := client.GetCosts(context.Background(), CostQuery{
		Start:    time.Date(2023, 10, 1, 0, 0, 0, 0, time.UTC),
		End:      time.Date(2023, 11, 1, 0, 0, 0, 0, time.UTC),
		GroupBy:  []string{"Account", "Service"},
		Filters:  map[string][]string{"CloudProvider": {"AWS"}},
		CostType: "real_cost",
	})
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, 1.5, rows[0].Cost)
	assert.Equal(t, "123", rows[0].Dimensions["Account"])
	assert.Equal(t, 2.0, rows[1].Cost)
	assert.Equal(t, int32(2), hits.Load())
	assert.Equal(t, 1, client.RequestsUsed())
}

func TestGetCostsRestartsOnceOn410(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusGone)
			writeRaw(t, w, `{"error":{"message":"expired","type":"ExpiredCache"}}`)
			return
		}
		writeJSON(t, w, map[string]any{
			"costs":      []map[string]any{{"usage_date": "2023-10-02T00:00:00Z", "cost": 4}},
			"pagination": map[string]any{"cursor": map[string]any{"has_next": false}},
		})
	}))
	t.Cleanup(srv.Close)
	client := testClient(t, srv.URL, "key", 10)
	rows, err := client.GetCosts(context.Background(), CostQuery{Start: time.Now().UTC(), CostType: "real_cost"})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, 2, client.RequestsUsed())
}

func TestGetCostsMapsStatuses(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status int
		body   string
		kind   Kind
	}{
		{"bad request", 400, `{"error":{"message":"bad","type":"Invalid"}}`, KindBadRequest},
		{"forbidden message", 403, `{"message":"nope"}`, KindAuth},
		{"unauthorized", 401, `{"error":{"message":"nope","type":"Auth"}}`, KindAuth},
		{"rate limit", 429, `{"error":{"message":"slow","type":"Rate"}}`, KindRateLimit},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			var hits atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				hits.Add(1)
				w.WriteHeader(tt.status)
				writeRaw(t, w, tt.body)
			}))
			t.Cleanup(srv.Close)
			client := testClient(t, srv.URL, "key", 10)
			_, err := client.GetCosts(context.Background(), CostQuery{Start: time.Now().UTC(), CostType: "real_cost"})
			var api *APIError
			require.ErrorAs(t, err, &api)
			assert.Equal(t, tt.kind, api.Kind)
			assert.NotContains(t, err.Error(), "nope")
			assert.NotContains(t, err.Error(), "bad")
			assert.Equal(t, int32(1), hits.Load())
		})
	}
}

func TestRateLimitCooldownSkipsRequests(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			w.WriteHeader(http.StatusTooManyRequests)
			writeRaw(t, w, `{"error":{"message":"slow","type":"Rate"}}`)
			return
		}
		writeJSON(t, w, emptyCosts())
	}))
	t.Cleanup(srv.Close)
	client := testClient(t, srv.URL, "key", 10)
	limitedAt := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	now := limitedAt
	client.now = func() time.Time { return now }
	query := CostQuery{Start: limitedAt, CostType: "real_cost"}

	_, err := client.GetCosts(context.Background(), query)
	var api *APIError
	require.ErrorAs(t, err, &api)
	assert.Equal(t, KindRateLimit, api.Kind)
	assert.Equal(t, 1, client.RequestsUsed())

	for _, at := range []time.Duration{0, time.Minute, rateLimitCooldown - time.Second} {
		now = limitedAt.Add(at)
		_, err = client.GetCosts(context.Background(), query)
		require.ErrorAs(t, err, &api, "after %s", at)
		assert.Equal(t, KindRateLimit, api.Kind)
		_, err = client.GetDimensions(context.Background())
		require.ErrorAs(t, err, &api, "after %s", at)
		assert.Equal(t, KindRateLimit, api.Kind)
	}
	assert.Equal(t, int32(1), hits.Load())
	assert.Equal(t, 1, client.RequestsUsed())

	now = limitedAt.Add(rateLimitCooldown + time.Second)
	rows, err := client.GetCosts(context.Background(), query)
	require.NoError(t, err)
	assert.Empty(t, rows)
	assert.Equal(t, int32(2), hits.Load())
	assert.Equal(t, 2, client.RequestsUsed())
}

func TestRateLimitCooldownStillSendsCursorPages(t *testing.T) {
	t.Parallel()
	var (
		hits   atomic.Int32
		client *Client
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Query().Get("cursor") == "" {
			client.coolDown()
			writeJSON(t, w, map[string]any{
				"costs":      []map[string]any{{"usage_date": "2026-11-01T00:00:00Z", "cost": 1}},
				"pagination": map[string]any{"cursor": map[string]any{"next_cursor": "page-2", "has_next": true}},
			})
			return
		}
		writeJSON(t, w, map[string]any{
			"costs":      []map[string]any{{"usage_date": "2026-11-02T00:00:00Z", "cost": 2}},
			"pagination": map[string]any{"cursor": map[string]any{"next_cursor": nil, "has_next": false}},
		})
	}))
	t.Cleanup(srv.Close)
	client = testClient(t, srv.URL, "key", 10)

	rows, err := client.GetCosts(context.Background(), CostQuery{CostType: "real_cost"})
	require.NoError(t, err, "a cursor page is sent during another query's cooldown")
	assert.Len(t, rows, 2)
	assert.Equal(t, int32(2), hits.Load())
	assert.Equal(t, 1, client.RequestsUsed())

	_, err = client.GetCosts(context.Background(), CostQuery{CostType: "real_cost"})
	var api *APIError
	require.ErrorAs(t, err, &api)
	assert.Equal(t, KindRateLimit, api.Kind, "a new first page is not sent during the cooldown")
	assert.Equal(t, int32(2), hits.Load())
}

func TestGetCostsRetriesServerError(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) < 3 {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		writeJSON(t, w, emptyCosts())
	}))
	t.Cleanup(srv.Close)
	client := testClient(t, srv.URL, "key", 10)
	rows, err := client.GetCosts(context.Background(), CostQuery{Start: time.Now().UTC(), CostType: "real_cost"})
	require.NoError(t, err)
	assert.Empty(t, rows)
	assert.Equal(t, int32(3), hits.Load())
	assert.Equal(t, 3, client.RequestsUsed())
}

func TestRetriesStopAtTheBudget(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)
	client := testClient(t, srv.URL, "key", 2)
	_, err := client.GetCosts(context.Background(), CostQuery{Start: time.Now().UTC(), CostType: "real_cost"})
	var api *APIError
	require.ErrorAs(t, err, &api)
	assert.Equal(t, KindBudget, api.Kind)
	assert.Equal(t, int32(2), hits.Load())
	assert.Equal(t, 2, client.RequestsUsed())
}

func TestGetCostsMalformedBody(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeRaw(t, w, `not-json`)
	}))
	t.Cleanup(srv.Close)
	client := testClient(t, srv.URL, "key", 10)
	_, err := client.GetCosts(context.Background(), CostQuery{Start: time.Now().UTC(), CostType: "real_cost"})
	var api *APIError
	require.ErrorAs(t, err, &api)
	assert.Equal(t, KindMalformed, api.Kind)
}

func TestBudgetStopsBeforeTheCall(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		writeJSON(t, w, emptyCosts())
	}))
	t.Cleanup(srv.Close)
	client := testClient(t, srv.URL, "key", 1)
	_, err := client.GetCosts(context.Background(), CostQuery{Start: time.Now().UTC(), CostType: "real_cost"})
	require.NoError(t, err)
	_, err = client.GetCosts(context.Background(), CostQuery{Start: time.Now().UTC(), CostType: "real_cost"})
	var api *APIError
	require.ErrorAs(t, err, &api)
	assert.Equal(t, KindBudget, api.Kind)
	assert.Equal(t, int32(1), hits.Load())
}

func TestMissingKeyDoesNotCall(t *testing.T) {
	t.Parallel()
	client := testClient(t, "http://127.0.0.1:1", "", 5)
	_, err := client.GetCosts(context.Background(), CostQuery{Start: time.Now().UTC()})
	var api *APIError
	require.ErrorAs(t, err, &api)
	assert.Equal(t, KindAuth, api.Kind)
	assert.Equal(t, 0, client.RequestsUsed())
}

func TestGetDimensions(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/billing/dimensions", r.URL.Path)
		writeJSON(t, w, map[string]any{"dimensions": []map[string]string{{"id": "Account", "name": "Account"}}})
	}))
	t.Cleanup(srv.Close)
	client := testClient(t, srv.URL, "key", 5)
	dims, err := client.GetDimensions(context.Background())
	require.NoError(t, err)
	require.Len(t, dims, 1)
	assert.Equal(t, "Account", dims[0].ID)
}

func TestKeyNeverAppearsInLogs(t *testing.T) {
	t.Parallel()
	const key = "cz-test-key-not-for-logs"
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		writeRaw(t, w, `{"message":"`+key+`"}`)
	}))
	t.Cleanup(srv.Close)
	client := New(config.Config{APIKey: key, BaseURL: srv.URL, DailyBudget: 5}, "vtest", logger)
	client.sleep = func(time.Duration) {}
	_, err := client.GetCosts(context.Background(), CostQuery{Start: time.Now().UTC(), CostType: "real_cost"})
	require.Error(t, err)
	assert.NotContains(t, err.Error(), key)
	assert.NotContains(t, buf.String(), key)
}

func TestTransportRetry(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	var buf bytes.Buffer
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, emptyCosts())
	}))
	t.Cleanup(srv.Close)
	client := testClient(t, srv.URL, "key", 5)
	client.logger = zerolog.New(&buf)
	base := client.httpClient.Transport
	if base == nil {
		base = http.DefaultTransport
	}
	client.httpClient.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if hits.Add(1) == 1 {
			return nil, errors.New("connection reset")
		}
		return base.RoundTrip(r)
	})
	rows, err := client.GetCosts(context.Background(), CostQuery{
		Start: time.Now().UTC(), CostType: "real_cost",
		Filters: map[string][]string{"Account": {"123456789012"}},
	})
	require.NoError(t, err)
	assert.Empty(t, rows)
	assert.Equal(t, int32(2), hits.Load())
	assert.Equal(t, 2, client.RequestsUsed())
	assert.Contains(t, buf.String(), "connection reset")
	assert.NotContains(t, buf.String(), "123456789012")
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testClient(t *testing.T, base, key string, budget int) *Client {
	t.Helper()
	client := New(config.Config{APIKey: key, BaseURL: base, DailyBudget: budget}, "vtest", zerolog.Nop())
	client.sleep = func(time.Duration) {}
	return client
}

func emptyCosts() map[string]any {
	return map[string]any{
		"costs":      []any{},
		"pagination": map[string]any{"cursor": map[string]any{"has_next": false}},
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, v any) {
	t.Helper()
	require.NoError(t, json.NewEncoder(w).Encode(v))
}

func writeRaw(t *testing.T, w io.Writer, body string) {
	t.Helper()
	_, err := io.WriteString(w, body)
	require.NoError(t, err)
}
