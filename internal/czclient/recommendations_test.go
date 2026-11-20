package czclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseRecommendations(t *testing.T) {
	t.Parallel()

	t.Run("sanitized live response", func(t *testing.T) {
		t.Parallel()
		body, err := os.ReadFile(filepath.Join("testdata", "live", "recommendations.json"))
		require.NoError(t, err)
		recs, next, hasNext, err := parseRecommendations(body)
		require.NoError(t, err)
		assert.Equal(t, "opaque-cursor-page-2", next)
		assert.True(t, hasNext)
		require.Len(t, recs, 5)
		for _, rec := range recs {
			assert.True(
				t,
				strings.HasPrefix(rec.Resource, "czrn:aws:ec2:us-west-2:123456789012:volume:vol-"),
				rec.Resource,
			)
			assert.Positive(t, rec.CostImpact)
		}
		assert.Equal(t, Recommendation{
			ID:             "CIR-AWS-00010-0000000000000000000000000000000000000001",
			Title:          "Upgradable EBS Volumes",
			TypeID:         "CIR-AWS-00010",
			TypeName:       "Upgradable EBS Volumes",
			Effort:         "low",
			Status:         "not_started",
			Resource:       "czrn:aws:ec2:us-west-2:123456789012:volume:vol-00000000000000001",
			ResourceType:   "volume",
			ResourceName:   "vol-00000000000000001",
			CostImpact:     0.33757025732400003,
			Created:        time.Unix(1794943043, 0).UTC(),
			Accounts:       []string{"123456789012"},
			Regions:        []string{"us-west-2"},
			CloudProviders: []string{"AWS"},
		}, recs[0])
	})

	t.Run("rows without an id or resource are dropped", func(t *testing.T) {
		t.Parallel()
		recs, _, hasNext, err := parseRecommendations([]byte(`{"recommendations":[
			{"recommendation_id":"no-resource","resource":null},
			{"recommendation_id":"","resource":"czrn:aws:ec2:us-west-2:123456789012:volume:vol-1"},
			{"recommendation_id":"kept","resource":"czrn:aws:ec2:us-west-2:123456789012:volume:vol-2",
				"cost_impact_last_30_days":"0.25","account":null,"region":null,"cloud_provider":null}]}`))
		require.NoError(t, err)
		assert.False(t, hasNext)
		require.Len(t, recs, 1)
		assert.Equal(t, "kept", recs[0].ID)
		assert.InDelta(t, 0.25, recs[0].CostImpact, 1e-9)
		assert.Empty(t, recs[0].Accounts)
		assert.Empty(t, recs[0].Regions)
		assert.Empty(t, recs[0].CloudProviders)
		assert.True(t, recs[0].Created.IsZero())
	})

	t.Run("body is not JSON", func(t *testing.T) {
		t.Parallel()
		_, _, _, err := parseRecommendations([]byte("<html>"))
		var api *APIError
		require.ErrorAs(t, err, &api)
		assert.Equal(t, KindMalformed, api.Kind)
	})
}

func TestGetRecommendationsQueryAndPages(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		query := r.URL.Query()
		assert.Equal(t, "/v2/optimize/recommendations", r.URL.Path)
		assert.Equal(t, "0.01", query.Get("cost_impact_last_30_days[gte]"))
		assert.Equal(t, []string{"not_started", "in_progress", "ignored"}, query["status"])
		if query.Get("cursor") == "" {
			writeRaw(t, w, `{"recommendations":[{"recommendation_id":"r1","resource":"czrn:a"}],
				"pagination":{"cursor":{"next_cursor":"page-2","has_next":true}}}`)
			return
		}
		assert.Equal(t, "page-2", query.Get("cursor"))
		writeRaw(t, w, `{"recommendations":[{"recommendation_id":"r2","resource":"czrn:b"}],
			"pagination":{"cursor":{"next_cursor":null,"has_next":false}}}`)
	}))
	t.Cleanup(srv.Close)

	client := testClient(t, srv.URL+"/v2", "key", 10)
	recs, err := client.GetRecommendations(context.Background())
	require.NoError(t, err)
	require.Len(t, recs, 2)
	assert.Equal(t, "r1", recs[0].ID)
	assert.Equal(t, "r2", recs[1].ID)
	assert.Equal(t, int32(2), hits.Load())
	assert.Equal(t, 1, client.RequestsUsed(), "only the first page spends budget")
}

func TestGetRecommendationsRespectsCooldown(t *testing.T) {
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

	_, err := client.GetBudgets(context.Background())
	var api *APIError
	require.ErrorAs(t, err, &api)
	assert.Equal(t, KindRateLimit, api.Kind)

	_, err = client.GetRecommendations(context.Background())
	require.ErrorAs(t, err, &api)
	assert.Equal(t, KindRateLimit, api.Kind)
	assert.Equal(t, int32(1), hits.Load(), "the cooldown sends nothing")
	assert.Equal(t, 1, client.RequestsUsed())
}
