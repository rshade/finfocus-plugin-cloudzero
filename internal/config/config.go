// Package config loads the CloudZero plugin settings from the environment.
package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	// DefaultBaseURL is the CloudZero REST API v2 origin, including the version prefix.
	DefaultBaseURL = "https://api.cloudzero.com/v2"
	// DefaultCostType is CloudZero's documented default. It is consumption spend,
	// and it drops tax, support, and unused commitments.
	DefaultCostType = "real_cost"
	// DefaultCurrency is an assumption. The costs response has no currency field.
	DefaultCurrency = "USD"
	// DefaultDailyBudget stays under the documented 60 requests a day.
	DefaultDailyBudget = 50
	// DefaultCacheTTL is how long a closed month of costs stays cached.
	DefaultCacheTTL = 6 * time.Hour
	// DefaultRecentTTL is how long a cost row from the newest two days stays cached.
	DefaultRecentTTL = time.Hour

	envAPIKey            = "CLOUDZERO_API_KEY" //nolint:gosec // environment variable name, not a credential
	envBaseURL           = "CLOUDZERO_BASE_URL"
	envCostType          = "CLOUDZERO_COST_TYPE"
	envCurrency          = "CLOUDZERO_CURRENCY"
	envTagDimensions     = "CLOUDZERO_TAG_DIMENSIONS"
	envResourceDimension = "CLOUDZERO_RESOURCE_DIMENSION"
	envDailyBudget       = "CLOUDZERO_DAILY_BUDGET"
	envCacheTTL          = "CLOUDZERO_CACHE_TTL"
	envRecentTTL         = "CLOUDZERO_RECENT_TTL"
)

// costTypes are the eight values the costs endpoint documents for cost_type.
var costTypes = map[string]struct{}{
	"billed_cost": {}, "discounted_cost": {}, "amortized_cost": {},
	"discounted_amortized_cost": {}, "real_cost": {}, "on_demand_cost": {},
	"invoiced_amortized_cost": {}, "usage_amount": {},
}

// Config is everything the plugin reads from its process environment.
type Config struct {
	APIKey            string
	BaseURL           string
	CostType          string
	Currency          string
	TagDimensions     map[string]string
	ResourceDimension string
	DailyBudget       int
	CacheTTL          time.Duration
	RecentTTL         time.Duration
}

// Load reads configuration from the environment. A missing API key is allowed:
// the process still starts, and cost RPCs report the missing key.
func Load() (Config, error) {
	cfg := Config{
		APIKey:            strings.TrimSpace(os.Getenv(envAPIKey)),
		BaseURL:           strings.TrimRight(strings.TrimSpace(os.Getenv(envBaseURL)), "/"),
		CostType:          strings.TrimSpace(os.Getenv(envCostType)),
		Currency:          strings.TrimSpace(os.Getenv(envCurrency)),
		TagDimensions:     map[string]string{},
		ResourceDimension: strings.TrimSpace(os.Getenv(envResourceDimension)),
		DailyBudget:       DefaultDailyBudget,
		CacheTTL:          DefaultCacheTTL,
		RecentTTL:         DefaultRecentTTL,
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = DefaultBaseURL
	}
	if cfg.CostType == "" {
		cfg.CostType = DefaultCostType
	}
	if _, ok := costTypes[cfg.CostType]; !ok {
		return Config{}, fmt.Errorf("CLOUDZERO_COST_TYPE %q is not a documented cost type", cfg.CostType)
	}
	if cfg.Currency == "" {
		cfg.Currency = DefaultCurrency
	}
	tags, err := parseTagDimensions(os.Getenv(envTagDimensions))
	if err != nil {
		return Config{}, err
	}
	cfg.TagDimensions = tags
	if err = applyBudget(&cfg, os.Getenv(envDailyBudget)); err != nil {
		return Config{}, err
	}
	if err = applyTTL(&cfg.CacheTTL, envCacheTTL, os.Getenv(envCacheTTL)); err != nil {
		return Config{}, err
	}
	if err = applyTTL(&cfg.RecentTTL, envRecentTTL, os.Getenv(envRecentTTL)); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// HasAPIKey reports whether a cost call can authenticate.
func (c Config) HasAPIKey() bool {
	return c.APIKey != ""
}

func parseTagDimensions(raw string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]string{}, nil
	}
	out := make(map[string]string)
	for part := range strings.SplitSeq(raw, ",") {
		key, dimension, ok := strings.Cut(strings.TrimSpace(part), "=")
		key = strings.TrimSpace(key)
		dimension = strings.TrimSpace(dimension)
		if !ok || key == "" || dimension == "" {
			return nil, fmt.Errorf("CLOUDZERO_TAG_DIMENSIONS entry %q must be tagKey=dimensionID", part)
		}
		out[key] = dimension
	}
	return out, nil
}

func applyBudget(cfg *Config, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 {
		return errors.New("CLOUDZERO_DAILY_BUDGET must be a positive integer")
	}
	cfg.DailyBudget = n
	return nil
}

func applyTTL(dest *time.Duration, name, raw string) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 {
		return fmt.Errorf("%s must be a positive duration", name)
	}
	*dest = d
	return nil
}
