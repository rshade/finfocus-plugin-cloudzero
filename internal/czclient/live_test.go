package czclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The bodies in testdata/live were captured from the CloudZero API on
// 2026-11-17.
func TestLiveErrorBodies(t *testing.T) {
	t.Parallel()
	tests := []struct {
		file   string
		status int
		want   Kind
	}{
		{"rate_limit_429.json", http.StatusTooManyRequests, KindRateLimit},
		{"bad_key_403.json", http.StatusForbidden, KindAuth},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			t.Parallel()
			body, err := os.ReadFile(filepath.Join("testdata", "live", tt.file))
			require.NoError(t, err)
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				writeRaw(t, w, string(body))
			}))
			t.Cleanup(srv.Close)
			_, err = testClient(t, srv.URL, "key", 5).GetCosts(
				context.Background(), CostQuery{Start: time.Now().UTC(), CostType: "real_cost"},
			)
			var api *APIError
			require.ErrorAs(t, err, &api)
			assert.Equal(t, tt.want, api.Kind)
			assert.NotContains(t, err.Error(), "identity-based policy")
			assert.NotContains(t, err.Error(), "minute(s)")
		})
	}
}
