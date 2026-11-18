package config

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDefaults(t *testing.T) {
	clearConfigEnv(t)
	cfg, err := Load()
	require.NoError(t, err)
	assert.Empty(t, cfg.APIKey)
	assert.False(t, cfg.HasAPIKey())
	assert.Equal(t, DefaultBaseURL, cfg.BaseURL)
	assert.Equal(t, DefaultCostType, cfg.CostType)
	assert.Equal(t, DefaultCurrency, cfg.Currency)
	assert.Empty(t, cfg.TagDimensions)
	assert.Empty(t, cfg.ResourceDimension)
	assert.Equal(t, DefaultDailyBudget, cfg.DailyBudget)
	assert.Equal(t, DefaultCacheTTL, cfg.CacheTTL)
	assert.Equal(t, DefaultRecentTTL, cfg.RecentTTL)
}

func TestLoadEveryVariable(t *testing.T) {
	clearConfigEnv(t)
	const key = "cz-test-key-not-for-logs"
	t.Setenv(envAPIKey, key)
	t.Setenv(envBaseURL, "https://cz.example/v2/")
	t.Setenv(envCostType, "billed_cost")
	t.Setenv(envCurrency, "EUR")
	t.Setenv(envTagDimensions, "Team=User:Defined:Team, Env=Environment")
	t.Setenv(envResourceDimension, "Resource")
	t.Setenv(envDailyBudget, "3")
	t.Setenv(envCacheTTL, "2h")
	t.Setenv(envRecentTTL, "15m")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, key, cfg.APIKey)
	assert.True(t, cfg.HasAPIKey())
	assert.Equal(t, "https://cz.example/v2", cfg.BaseURL)
	assert.Equal(t, "billed_cost", cfg.CostType)
	assert.Equal(t, "EUR", cfg.Currency)
	assert.Equal(t, map[string]string{"Team": "User:Defined:Team", "Env": "Environment"}, cfg.TagDimensions)
	assert.Equal(t, "Resource", cfg.ResourceDimension)
	assert.Equal(t, 3, cfg.DailyBudget)
	assert.Equal(t, 2*time.Hour, cfg.CacheTTL)
	assert.Equal(t, 15*time.Minute, cfg.RecentTTL)
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	const key = "cz-test-key-not-for-logs"
	tests := []struct {
		name string
		env  string
		val  string
	}{
		{"cost type", envCostType, "invoice"},
		{"tag map", envTagDimensions, "Team"},
		{"budget", envDailyBudget, "0"},
		{"cache ttl", envCacheTTL, "nope"},
		{"recent ttl", envRecentTTL, "-1s"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv(envAPIKey, key)
			t.Setenv(tt.env, tt.val)
			_, err := Load()
			require.Error(t, err)
			assert.NotContains(t, err.Error(), key)
		})
	}
}

func TestLoadAcceptsDocumentedCostTypes(t *testing.T) {
	for name := range costTypes {
		t.Run(name, func(t *testing.T) {
			clearConfigEnv(t)
			t.Setenv(envCostType, name)
			cfg, err := Load()
			require.NoError(t, err)
			assert.Equal(t, name, cfg.CostType)
		})
	}
}

func clearConfigEnv(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		envAPIKey, envBaseURL, envCostType, envCurrency, envTagDimensions,
		envResourceDimension, envDailyBudget, envCacheTTL, envRecentTTL,
	} {
		t.Setenv(name, "")
	}
}

func TestParseTagDimensionsRejectsEmptySide(t *testing.T) {
	_, err := parseTagDimensions("=Dimension")
	require.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "tagKey=dimensionID"))
}
