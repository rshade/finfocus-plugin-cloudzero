package plugin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rshade/finfocus-plugin-cloudzero/internal/config"
	"github.com/rshade/finfocus-plugin-cloudzero/internal/czclient"
)

const (
	firstARN   = "arn:aws:ec2:us-east-1:123456789012:instance/i-1"
	firstCZRN  = "czrn:aws:ec2:us-east-1:123456789012:instance:i-1"
	secondARN  = "arn:aws:ec2:us-east-1:123456789012:instance/i-2"
	secondCZRN = "czrn:aws:ec2:us-east-1:123456789012:instance:i-2"
)

func TestInfoDeclaresActualAndBatchOnly(t *testing.T) {
	t.Parallel()
	info := Info("0.1.0")
	assert.Equal(t, []pbc.PluginCapability{
		pbc.PluginCapability_PLUGIN_CAPABILITY_ACTUAL_COSTS,
		pbc.PluginCapability_PLUGIN_CAPABILITY_BATCH_COST,
	}, info.Capabilities)
	logger := zerolog.Nop()
	server := pluginsdk.NewServerWithOptions(fakePlugin(config.Config{}, &fakeCosts{}), nil, &logger, info)
	got, err := server.GetPluginInfo(context.Background(), &pbc.GetPluginInfoRequest{})
	require.NoError(t, err)
	assert.Equal(t, info.Capabilities, got.GetCapabilities())
	assert.Equal(t, pluginName, fakePlugin(config.Config{}, &fakeCosts{}).Name())
}

func TestSupports(t *testing.T) {
	t.Parallel()
	ready := fakePlugin(readyConfig(), &fakeCosts{})
	aws := &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "aws:ec2/instance:Instance"}
	resp, err := ready.Supports(context.Background(), &pbc.SupportsRequest{
		Resource: aws, Capability: pbc.PluginCapability_PLUGIN_CAPABILITY_ACTUAL_COSTS,
	})
	require.NoError(t, err)
	assert.True(t, resp.GetSupported())

	tests := []struct {
		name string
		plug *CloudZeroPlugin
		req  *pbc.SupportsRequest
	}{
		{"nil", ready, nil},
		{"other provider", ready, &pbc.SupportsRequest{Resource: &pbc.ResourceDescriptor{Provider: "kubernetes"}}},
		{
			"projected",
			ready,
			&pbc.SupportsRequest{Resource: aws, Capability: pbc.PluginCapability_PLUGIN_CAPABILITY_PROJECTED_COSTS},
		},
		{"unspecified", ready, &pbc.SupportsRequest{Resource: aws}},
		{"no key", fakePlugin(config.Config{ResourceDimension: "Resource"}, &fakeCosts{}), &pbc.SupportsRequest{
			Resource: aws, Capability: pbc.PluginCapability_PLUGIN_CAPABILITY_ACTUAL_COSTS,
		}},
		{"no dimension", fakePlugin(config.Config{APIKey: "k"}, &fakeCosts{}), &pbc.SupportsRequest{
			Resource: aws, Capability: pbc.PluginCapability_PLUGIN_CAPABILITY_BATCH_COST,
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := tt.plug.Supports(context.Background(), tt.req)
			require.NoError(t, err)
			assert.False(t, got.GetSupported())
			assert.NotEmpty(t, got.GetReason())
		})
	}
}

func TestUnimplementedCostRPCs(t *testing.T) {
	t.Parallel()
	plug := fakePlugin(readyConfig(), &fakeCosts{})
	_, err := plug.GetProjectedCost(context.Background(), &pbc.GetProjectedCostRequest{})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
	_, err = plug.GetPricingSpec(context.Background(), &pbc.GetPricingSpecRequest{})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
	_, err = plug.EstimateCost(context.Background(), &pbc.EstimateCostRequest{})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}

// fakeCosts records every query and answers with fixed rows or a fixed error.
type fakeCosts struct {
	mu      sync.Mutex
	rows    []czclient.CostRow
	err     error
	queries []czclient.CostQuery
}

func (f *fakeCosts) GetCosts(_ context.Context, q czclient.CostQuery) ([]czclient.CostRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, q)
	return f.rows, f.err
}

func (f *fakeCosts) calls() []czclient.CostQuery {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]czclient.CostQuery(nil), f.queries...)
}

func costRow(when time.Time, cost float64, resourceID string) czclient.CostRow {
	return czclient.CostRow{UsageDate: when, Cost: cost, Dimensions: map[string]string{"Resource": resourceID}}
}

func readyConfig() config.Config {
	return config.Config{
		APIKey: "test-key", CostType: "real_cost", Currency: "USD",
		ResourceDimension: "Resource", DailyBudget: 10,
		CacheTTL: time.Hour, RecentTTL: time.Minute,
	}
}

func fakePlugin(cfg config.Config, costs *fakeCosts) *CloudZeroPlugin {
	return New("vtest", cfg, costs, zerolog.Nop())
}

func httpPlugin(t *testing.T, cfg config.Config, handler http.HandlerFunc, logger zerolog.Logger) *CloudZeroPlugin {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	cfg.BaseURL = srv.URL
	return New("vtest", cfg, czclient.New(cfg, "vtest", logger), logger)
}

func actualReq(start, end time.Time, arn string) *pbc.GetActualCostRequest {
	return &pbc.GetActualCostRequest{
		Start: timestamppb.New(start), End: timestamppb.New(end), Resource: resource(arn),
	}
}

func resource(arn string) *pbc.ResourceDescriptor {
	return &pbc.ResourceDescriptor{Provider: "aws", Region: "us-east-1", Arn: arn}
}

func day(year int, month time.Month, d int) time.Time {
	return time.Date(year, month, d, 0, 0, 0, 0, time.UTC)
}
