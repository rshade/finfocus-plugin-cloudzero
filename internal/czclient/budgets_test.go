package czclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBudgets(t *testing.T) {
	t.Parallel()
	november := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

	t.Run("docs-shaped fixture", func(t *testing.T) {
		t.Parallel()
		body, err := os.ReadFile(filepath.Join("testdata", "budgets_docs.json"))
		require.NoError(t, err)
		budgets, next, hasNext, err := parseBudgets(body)
		require.NoError(t, err)
		assert.Empty(t, next)
		assert.False(t, hasNext)
		require.Len(t, budgets, 2)
		first := budgets[0]
		assert.Equal(t, "00000000-0000-0000-0000-00000000b001", first.ID)
		assert.Equal(t, "Platform monthly", first.Name)
		assert.Equal(t, "00000000-0000-0000-0000-00000000f001", first.ViewID)
		assert.Equal(t, "monthly", first.Granularity)
		assert.InDelta(t, 500.0, first.Monthly[november], 1e-9)
		assert.Len(t, first.Monthly, 2)
		assert.Equal(t, []float64{80}, first.Alerts)
		require.NotNil(t, first.Actual)
		assert.InDelta(t, 120.5, *first.Actual, 1e-9)
		require.NotNil(t, first.Planned)
		assert.InDelta(t, 500.0, *first.Planned, 1e-9)
		assert.Equal(t, "quarterly", budgets[1].Granularity)
	})

	t.Run("numbers and month keys", func(t *testing.T) {
		t.Parallel()
		budgets, _, _, err := parseBudgets([]byte(`{"budgets":[{"id":"b","name":"n",
			"planned_limits":{"monthly":{"2026-11":{"amount":250},"not-a-date":{"amount":1}}},
			"alerts":{"100":true,"50":"y","0":"Y","150":"Y","x":"Y"},
			"current":{"planned":250,"actual":12.25}}]}`))
		require.NoError(t, err)
		require.Len(t, budgets, 1)
		assert.Equal(t, map[time.Time]float64{november: 250}, budgets[0].Monthly)
		assert.Equal(t, []float64{50, 100}, budgets[0].Alerts)
		require.NotNil(t, budgets[0].Actual)
		assert.InDelta(t, 12.25, *budgets[0].Actual, 1e-9)
		assert.Empty(t, budgets[0].ViewID)
	})

	t.Run("unparsable amounts are left unset", func(t *testing.T) {
		t.Parallel()
		budgets, _, _, err := parseBudgets([]byte(`{"budgets":[{"id":"b","name":"n",
			"planned_limits":{"monthly":{"2026-11-01":{"amount":"lots"}}},
			"current":{"planned":null,"actual":"n/a"}}]}`))
		require.NoError(t, err)
		require.Len(t, budgets, 1)
		assert.Equal(t, "b", budgets[0].ID)
		assert.Empty(t, budgets[0].Monthly)
		assert.Nil(t, budgets[0].Planned)
		assert.Nil(t, budgets[0].Actual)
	})

	t.Run("body is not JSON", func(t *testing.T) {
		t.Parallel()
		_, _, _, err := parseBudgets([]byte("<html>"))
		var api *APIError
		require.ErrorAs(t, err, &api)
		assert.Equal(t, KindMalformed, api.Kind)
	})
}

func TestGetBudgetsPagesAndCharges(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		assert.Equal(t, "/v2/budgets", r.URL.Path)
		assert.Equal(t, "current", r.URL.Query().Get("expand"))
		if r.URL.Query().Get("cursor") == "" {
			writeRaw(t, w, `{"budgets":[{"id":"b1","name":"one"}],
				"pagination":{"cursor":{"next_cursor":"page-2","has_next":true}}}`)
			return
		}
		assert.Equal(t, "page-2", r.URL.Query().Get("cursor"))
		writeRaw(t, w, `{"budgets":[{"id":"b2","name":"two"}],
			"pagination":{"cursor":{"next_cursor":null,"has_next":false}}}`)
	}))
	t.Cleanup(srv.Close)

	client := testClient(t, srv.URL+"/v2", "key", 10)
	budgets, err := client.GetBudgets(context.Background())
	require.NoError(t, err)
	require.Len(t, budgets, 2)
	assert.Equal(t, "b1", budgets[0].ID)
	assert.Equal(t, "b2", budgets[1].ID)
	assert.Equal(t, int32(2), hits.Load())
	assert.Equal(t, 1, client.RequestsUsed())
}

func TestGetBudgetsRespectsCooldown(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
		writeRaw(t, w, `{"error":{"message":"slow","type":"Rate"}}`)
	}))
	t.Cleanup(srv.Close)
	client := testClient(t, srv.URL, "key", 10)
	limitedAt := time.Date(2026, 11, 19, 12, 0, 0, 0, time.UTC)
	client.now = func() time.Time { return limitedAt }

	_, err := client.GetCosts(context.Background(), CostQuery{Start: limitedAt, CostType: "real_cost"})
	var api *APIError
	require.ErrorAs(t, err, &api)
	assert.Equal(t, KindRateLimit, api.Kind)

	_, err = client.GetBudgets(context.Background())
	require.ErrorAs(t, err, &api)
	assert.Equal(t, KindRateLimit, api.Kind)
	assert.Equal(t, int32(1), hits.Load())
	assert.Equal(t, 1, client.RequestsUsed())
}
