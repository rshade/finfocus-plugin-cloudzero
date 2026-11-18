//go:build integration

// Package integration calls the live CloudZero API. It skips unless
// CLOUDZERO_API_KEY is set, and it makes at most three calls.
package integration

import (
	"context"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/require"

	"github.com/rshade/finfocus-plugin-cloudzero/internal/config"
	"github.com/rshade/finfocus-plugin-cloudzero/internal/czclient"
)

func TestLiveCosts(t *testing.T) {
	cfg, err := config.Load()
	require.NoError(t, err)
	if !cfg.HasAPIKey() {
		t.Skip("set CLOUDZERO_API_KEY")
	}
	client := czclient.New(cfg, "integration", zerolog.Nop())
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, err = client.GetDimensions(ctx)
	require.NoError(t, err)
	end := time.Now().UTC().Truncate(24 * time.Hour)
	start := end.AddDate(0, 0, -2)
	_, err = client.GetCosts(ctx, czclient.CostQuery{
		Start: start, End: end, CostType: cfg.CostType, GroupBy: []string{"Service"},
	})
	require.NoError(t, err)
	require.LessOrEqual(t, client.RequestsUsed(), 3)
}
