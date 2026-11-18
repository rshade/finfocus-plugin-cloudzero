package czclient

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAPIErrorText(t *testing.T) {
	t.Parallel()
	cases := []Kind{
		KindAuth, KindBadRequest, KindRateLimit, KindExpired, KindBudget, KindMalformed, KindUnavailable, Kind(99),
	}
	for _, kind := range cases {
		assert.NotEmpty(t, newAPIError(kind, 0).Error())
	}
	assert.Equal(t, KindUnavailable, statusKind(http.StatusBadGateway))
	assert.Equal(t, KindBadRequest, statusKind(http.StatusBadRequest))
}

func TestFloatValue(t *testing.T) {
	t.Parallel()
	got, err := floatValue(json.Number("1.25"))
	require.NoError(t, err)
	assert.InDelta(t, 1.25, got, 0.0001)
	got, err = floatValue("2.5")
	require.NoError(t, err)
	assert.InDelta(t, 2.5, got, 0.0001)
	_, err = floatValue("nope")
	require.Error(t, err)
	_, err = floatValue(true)
	require.Error(t, err)
}

func TestParseRejectsBadRows(t *testing.T) {
	t.Parallel()
	_, _, _, err := parseCosts([]byte(`{"costs":[{"usage_date":"yesterday","cost":1}]}`))
	var api *APIError
	require.ErrorAs(t, err, &api)
	assert.Equal(t, KindMalformed, api.Kind)
	_, _, _, err = parseCosts([]byte(`{"costs":[{"usage_date":"2023-10-02T00:00:00Z"}]}`))
	require.ErrorAs(t, err, &api)
	_, err = parseDimensions([]byte(`{`))
	require.ErrorAs(t, err, &api)
}

func TestGetDimensionsFailures(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		writeRaw(t, w, `{"message":"no"}`)
	}))
	t.Cleanup(srv.Close)
	_, err := testClient(t, srv.URL, "key", 5).GetDimensions(context.Background())
	var api *APIError
	require.ErrorAs(t, err, &api)
	assert.Equal(t, KindAuth, api.Kind)

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeRaw(t, w, `{`)
	}))
	t.Cleanup(bad.Close)
	_, err = testClient(t, bad.URL, "key", 5).GetDimensions(context.Background())
	require.ErrorAs(t, err, &api)
	assert.Equal(t, KindMalformed, api.Kind)
}

func TestRepeatedCursorAndServerGiveUp(t *testing.T) {
	t.Parallel()
	loop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(t, w, map[string]any{
			"costs": []any{},
			"pagination": map[string]any{"cursor": map[string]any{
				"next_cursor": "again", "has_next": true,
			}},
		})
	}))
	t.Cleanup(loop.Close)
	_, err := testClient(t, loop.URL, "key", 5).GetCosts(context.Background(), CostQuery{Start: time.Now().UTC()})
	var api *APIError
	require.ErrorAs(t, err, &api)
	assert.Equal(t, KindMalformed, api.Kind)

	var hits atomic.Int32
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(down.Close)
	_, err = testClient(t, down.URL, "key", 5).GetCosts(context.Background(), CostQuery{Start: time.Now().UTC()})
	require.ErrorAs(t, err, &api)
	assert.Equal(t, KindUnavailable, api.Kind)
	assert.Equal(t, int32(maxAttempts), hits.Load())
}

func TestCanceledContextDoesNotRetry(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		writeJSON(t, w, emptyCosts())
	}))
	t.Cleanup(srv.Close)
	client := testClient(t, srv.URL, "key", 5)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.GetCosts(ctx, CostQuery{Start: time.Now().UTC()})
	require.Error(t, err)
	assert.Zero(t, hits.Load())
	assert.False(t, errors.Is(err, context.Canceled))
}
