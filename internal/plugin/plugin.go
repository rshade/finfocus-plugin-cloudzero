// Package plugin serves the FinFocus cost source RPCs for CloudZero actual cost.
package plugin

import (
	"context"
	"time"

	"github.com/rs/zerolog"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/rshade/finfocus-plugin-cloudzero/internal/cache"
	"github.com/rshade/finfocus-plugin-cloudzero/internal/config"
	"github.com/rshade/finfocus-plugin-cloudzero/internal/czclient"
	"github.com/rshade/finfocus-plugin-cloudzero/internal/mapping"
)

const (
	pluginName       = "cloudzero"
	defaultBatchSize = 100
	keyNotSet        = "set CLOUDZERO_API_KEY"
)

// CostSource reads CloudZero costs.
type CostSource interface {
	GetCosts(ctx context.Context, q czclient.CostQuery) ([]czclient.CostRow, error)
}

// BudgetSource reads CloudZero budgets with their current spend.
type BudgetSource interface {
	GetBudgets(ctx context.Context) ([]czclient.Budget, error)
}

// RecommendationSource reads CloudZero's optimization recommendations.
type RecommendationSource interface {
	GetRecommendations(ctx context.Context) ([]czclient.Recommendation, error)
}

// CloudZero is every CloudZero read the plugin depends on. *czclient.Client
// implements it. Tests substitute a fake.
type CloudZero interface {
	CostSource
	BudgetSource
	RecommendationSource
}

// CloudZeroPlugin implements the cost RPCs CloudZero can answer.
type CloudZeroPlugin struct {
	pbc.UnimplementedCostSourceServiceServer

	version         string
	cfg             config.Config
	client          CloudZero
	logger          zerolog.Logger
	mapper          *mapping.Mapper
	cache           *cache.Cache
	costsInFlight   *inflight[[]czclient.CostRow]
	budgetsInFlight *inflight[[]czclient.Budget]
	recsInFlight    *inflight[[]czclient.Recommendation]
	now             func() time.Time
}

// New returns a plugin. It starts without an API key. Cost RPCs then fail
// with FailedPrecondition.
func New(version string, cfg config.Config, client CloudZero, logger zerolog.Logger) *CloudZeroPlugin {
	return &CloudZeroPlugin{
		UnimplementedCostSourceServiceServer: pbc.UnimplementedCostSourceServiceServer{},
		version:                              version,
		cfg:                                  cfg,
		client:                               client,
		logger:                               logger,
		mapper:                               mapping.NewMapper(logger, cfg.TagDimensions, cfg.ResourceDimension),
		cache:                                cache.NewCache(),
		costsInFlight:                        newInflight[[]czclient.CostRow](),
		budgetsInFlight:                      newInflight[[]czclient.Budget](),
		recsInFlight:                         newInflight[[]czclient.Recommendation](),
		now:                                  time.Now,
	}
}

// Name returns the plugin identifier the SDK reports to core.
func (p *CloudZeroPlugin) Name() string {
	return pluginName
}

// Info is the metadata served by GetPluginInfo. Capabilities are actual cost,
// batch actual cost, budgets, and recommendations.
func Info(version string) *pluginsdk.PluginInfo {
	return &pluginsdk.PluginInfo{
		Name:        pluginName,
		Version:     version,
		SpecVersion: pluginsdk.SpecVersion,
		Providers:   []string{"aws", "azure", "gcp"},
		Capabilities: []pbc.PluginCapability{
			pbc.PluginCapability_PLUGIN_CAPABILITY_ACTUAL_COSTS,
			pbc.PluginCapability_PLUGIN_CAPABILITY_BATCH_COST,
			pbc.PluginCapability_PLUGIN_CAPABILITY_BUDGETS,
			pbc.PluginCapability_PLUGIN_CAPABILITY_RECOMMENDATIONS,
		},
	}
}

// Supports accepts actual cost, batch cost, budgets, and recommendations for
// aws, azure, and gcp when a key is configured. Cost capabilities also need a
// resource dimension.
func (p *CloudZeroPlugin) Supports(_ context.Context, req *pbc.SupportsRequest) (*pbc.SupportsResponse, error) {
	if req == nil || req.GetResource() == nil {
		return decline("invalid resource descriptor"), nil
	}
	if !mapping.SupportedProvider(req.GetResource().GetProvider()) {
		return decline("provider is not aws, azure, or gcp"), nil
	}
	need := settingsFor(req.GetCapability())
	if need == unsupported {
		return decline("only actual cost, batch cost, budgets, and recommendations are supported"), nil
	}
	if !p.cfg.HasAPIKey() {
		return decline(keyNotSet), nil
	}
	if need == keyAndDimension {
		if missing := p.missingSetting(); missing != "" {
			return decline(missing), nil
		}
	}
	return &pbc.SupportsResponse{Supported: true}, nil
}

// requiredSettings is what Supports needs configured to accept a capability.
type requiredSettings int

const (
	unsupported requiredSettings = iota
	keyOnly
	// keyAndDimension is for the costs query, the only read that groups by
	// the resource dimension.
	keyAndDimension
)

func settingsFor(capability pbc.PluginCapability) requiredSettings {
	if capability == pbc.PluginCapability_PLUGIN_CAPABILITY_ACTUAL_COSTS ||
		capability == pbc.PluginCapability_PLUGIN_CAPABILITY_BATCH_COST {
		return keyAndDimension
	}
	if capability == pbc.PluginCapability_PLUGIN_CAPABILITY_BUDGETS ||
		capability == pbc.PluginCapability_PLUGIN_CAPABILITY_RECOMMENDATIONS {
		return keyOnly
	}
	return unsupported
}

// missingSetting names the first setting a cost query needs that is not set,
// or returns an empty string when the plugin is ready.
func (p *CloudZeroPlugin) missingSetting() string {
	if !p.cfg.HasAPIKey() {
		return keyNotSet
	}
	if p.cfg.ResourceDimension == "" {
		return "set CLOUDZERO_RESOURCE_DIMENSION to a resource-level dimension ID"
	}
	return ""
}

func decline(reason string) *pbc.SupportsResponse {
	return &pbc.SupportsResponse{Supported: false, Reason: reason}
}

// GetProjectedCost is not a CloudZero capability.
func (p *CloudZeroPlugin) GetProjectedCost(
	context.Context,
	*pbc.GetProjectedCostRequest,
) (*pbc.GetProjectedCostResponse, error) {
	return nil, status.Error(codes.Unimplemented, "CloudZero provides actual cost only")
}

// GetPricingSpec is not a CloudZero capability.
func (p *CloudZeroPlugin) GetPricingSpec(
	context.Context,
	*pbc.GetPricingSpecRequest,
) (*pbc.GetPricingSpecResponse, error) {
	return nil, status.Error(codes.Unimplemented, "CloudZero provides actual cost only")
}

// EstimateCost is not a CloudZero capability.
func (p *CloudZeroPlugin) EstimateCost(context.Context, *pbc.EstimateCostRequest) (*pbc.EstimateCostResponse, error) {
	return nil, status.Error(codes.Unimplemented, "CloudZero provides actual cost only")
}
