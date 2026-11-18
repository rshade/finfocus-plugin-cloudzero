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
)

// CostSource is the CloudZero call the plugin depends on. *czclient.Client
// implements it. Tests substitute a fake.
type CostSource interface {
	GetCosts(ctx context.Context, q czclient.CostQuery) ([]czclient.CostRow, error)
}

// CloudZeroPlugin implements the cost RPCs CloudZero can answer.
type CloudZeroPlugin struct {
	pbc.UnimplementedCostSourceServiceServer

	version string
	cfg     config.Config
	client  CostSource
	logger  zerolog.Logger
	mapper  *mapping.Mapper
	cache   *cache.Cache
	fetches *inflight
	now     func() time.Time
}

// New returns a plugin. It starts without an API key. Cost RPCs then fail
// with FailedPrecondition.
func New(version string, cfg config.Config, client CostSource, logger zerolog.Logger) *CloudZeroPlugin {
	return &CloudZeroPlugin{
		UnimplementedCostSourceServiceServer: pbc.UnimplementedCostSourceServiceServer{},
		version:                              version,
		cfg:                                  cfg,
		client:                               client,
		logger:                               logger,
		mapper:                               mapping.NewMapper(logger, cfg.TagDimensions, cfg.ResourceDimension),
		cache:                                cache.NewCache(),
		fetches:                              newInflight(),
		now:                                  time.Now,
	}
}

// Name returns the plugin identifier the SDK reports to core.
func (p *CloudZeroPlugin) Name() string {
	return pluginName
}

// Info is the metadata served by GetPluginInfo. Capabilities are only actual
// cost and batch actual cost.
func Info(version string) *pluginsdk.PluginInfo {
	return &pluginsdk.PluginInfo{
		Name:        pluginName,
		Version:     version,
		SpecVersion: pluginsdk.SpecVersion,
		Providers:   []string{"aws", "azure", "gcp"},
		Capabilities: []pbc.PluginCapability{
			pbc.PluginCapability_PLUGIN_CAPABILITY_ACTUAL_COSTS,
			pbc.PluginCapability_PLUGIN_CAPABILITY_BATCH_COST,
		},
	}
}

// Supports declines anything other than actual cost for aws, azure, and gcp
// when a key and a resource dimension are configured.
func (p *CloudZeroPlugin) Supports(_ context.Context, req *pbc.SupportsRequest) (*pbc.SupportsResponse, error) {
	if req == nil || req.GetResource() == nil {
		return decline("invalid resource descriptor"), nil
	}
	if !mapping.SupportedProvider(req.GetResource().GetProvider()) {
		return decline("provider is not aws, azure, or gcp"), nil
	}
	if !allowedCapability(req.GetCapability()) {
		return decline("only actual cost is supported"), nil
	}
	if missing := p.missingSetting(); missing != "" {
		return decline(missing), nil
	}
	return &pbc.SupportsResponse{Supported: true}, nil
}

// missingSetting names the first setting a cost query needs that is not set,
// or returns an empty string when the plugin is ready.
func (p *CloudZeroPlugin) missingSetting() string {
	if !p.cfg.HasAPIKey() {
		return "set CLOUDZERO_API_KEY"
	}
	if p.cfg.ResourceDimension == "" {
		return "set CLOUDZERO_RESOURCE_DIMENSION to a resource-level dimension ID"
	}
	return ""
}

func allowedCapability(capability pbc.PluginCapability) bool {
	return capability == pbc.PluginCapability_PLUGIN_CAPABILITY_ACTUAL_COSTS ||
		capability == pbc.PluginCapability_PLUGIN_CAPABILITY_BATCH_COST
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
