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

func TestInfoDeclaresCapabilities(t *testing.T) {
	t.Parallel()
	info := Info("0.1.0")
	assert.Equal(t, []pbc.PluginCapability{
		pbc.PluginCapability_PLUGIN_CAPABILITY_ACTUAL_COSTS,
		pbc.PluginCapability_PLUGIN_CAPABILITY_BATCH_COST,
		pbc.PluginCapability_PLUGIN_CAPABILITY_BUDGETS,
		pbc.PluginCapability_PLUGIN_CAPABILITY_RECOMMENDATIONS,
	}, info.Capabilities)
	logger := zerolog.Nop()
	server := pluginsdk.NewServerWithOptions(fakePlugin(config.Config{}, &fakeCloudZero{}), nil, &logger, info)
	got, err := server.GetPluginInfo(context.Background(), &pbc.GetPluginInfoRequest{})
	require.NoError(t, err)
	assert.Equal(t, info.Capabilities, got.GetCapabilities())
	assert.Equal(t, "cloudzero", got.GetName())
	assert.Equal(t, "cloudzero", fakePlugin(config.Config{}, &fakeCloudZero{}).Name())
}

func TestSupports(t *testing.T) {
	t.Parallel()
	ready := fakePlugin(readyConfig(), &fakeCloudZero{})
	aws := &pbc.ResourceDescriptor{Provider: "aws", ResourceType: "aws:ec2/instance:Instance"}
	resp, err := ready.Supports(context.Background(), &pbc.SupportsRequest{
		Resource: aws, Capability: pbc.PluginCapability_PLUGIN_CAPABILITY_ACTUAL_COSTS,
	})
	require.NoError(t, err)
	assert.True(t, resp.GetSupported())

	keyOnly := fakePlugin(config.Config{APIKey: "k"}, &fakeCloudZero{})
	for _, capability := range []pbc.PluginCapability{
		pbc.PluginCapability_PLUGIN_CAPABILITY_BUDGETS, pbc.PluginCapability_PLUGIN_CAPABILITY_RECOMMENDATIONS,
	} {
		resp, err = keyOnly.Supports(context.Background(), &pbc.SupportsRequest{Resource: aws, Capability: capability})
		require.NoError(t, err)
		assert.True(t, resp.GetSupported(), "%s needs only the key", capability)
	}

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
		{"no key", fakePlugin(config.Config{ResourceDimension: "Resource"}, &fakeCloudZero{}), &pbc.SupportsRequest{
			Resource: aws, Capability: pbc.PluginCapability_PLUGIN_CAPABILITY_ACTUAL_COSTS,
		}},
		{"no dimension", fakePlugin(config.Config{APIKey: "k"}, &fakeCloudZero{}), &pbc.SupportsRequest{
			Resource: aws, Capability: pbc.PluginCapability_PLUGIN_CAPABILITY_BATCH_COST,
		}},
		{"budgets without key", fakePlugin(config.Config{}, &fakeCloudZero{}), &pbc.SupportsRequest{
			Resource: aws, Capability: pbc.PluginCapability_PLUGIN_CAPABILITY_BUDGETS,
		}},
		{"recommendations without key", fakePlugin(config.Config{}, &fakeCloudZero{}), &pbc.SupportsRequest{
			Resource: aws, Capability: pbc.PluginCapability_PLUGIN_CAPABILITY_RECOMMENDATIONS,
		}},
		{
			"dry run",
			ready,
			&pbc.SupportsRequest{Resource: aws, Capability: pbc.PluginCapability_PLUGIN_CAPABILITY_DRY_RUN},
		},
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
	plug := fakePlugin(readyConfig(), &fakeCloudZero{})
	_, err := plug.GetProjectedCost(context.Background(), &pbc.GetProjectedCostRequest{})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
	_, err = plug.GetPricingSpec(context.Background(), &pbc.GetPricingSpecRequest{})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
	_, err = plug.EstimateCost(context.Background(), &pbc.EstimateCostRequest{})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
}

// fakeCloudZero records every cost query and budget and recommendation fetch
// and answers with fixed rows, budgets, recommendations, or errors. A non-nil gate holds each call open until
// the test closes it.
type fakeCloudZero struct {
	mu          sync.Mutex
	rows        []czclient.CostRow
	err         error
	queries     []czclient.CostQuery
	gate        chan struct{}
	budgets     []czclient.Budget
	budgetErr   error
	budgetCalls int
	recs        []czclient.Recommendation
	recErr      error
	recCalls    int
}

func (f *fakeCloudZero) GetRecommendations(ctx context.Context) ([]czclient.Recommendation, error) {
	f.mu.Lock()
	f.recCalls++
	recs, err, gate := f.recs, f.recErr, f.gate
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return recs, err
}

func (f *fakeCloudZero) recommendationFetches() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.recCalls
}

func (f *fakeCloudZero) GetBudgets(ctx context.Context) ([]czclient.Budget, error) {
	f.mu.Lock()
	f.budgetCalls++
	budgets, err, gate := f.budgets, f.budgetErr, f.gate
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return budgets, err
}

func (f *fakeCloudZero) budgetFetches() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.budgetCalls
}

func (f *fakeCloudZero) GetCosts(ctx context.Context, q czclient.CostQuery) ([]czclient.CostRow, error) {
	f.mu.Lock()
	f.queries = append(f.queries, q)
	rows, err, gate := f.rows, f.err, f.gate
	f.mu.Unlock()
	if gate != nil {
		select {
		case <-gate:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	return rows, err
}

func (f *fakeCloudZero) calls() []czclient.CostQuery {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]czclient.CostQuery(nil), f.queries...)
}

func costRow(when time.Time, cost float64, resourceID string) czclient.CostRow {
	return czclient.CostRow{
		UsageDate: when, Cost: cost,
		Dimensions: map[string]string{"Resource": resourceID, "CloudProvider": "AWS"},
	}
}

func readyConfig() config.Config {
	return config.Config{
		APIKey: "test-key", CostType: "real_cost", Currency: "USD",
		ResourceDimension: "Resource", DailyBudget: 10,
		CacheTTL: time.Hour, RecentTTL: time.Minute,
	}
}

func fakePlugin(cfg config.Config, costs *fakeCloudZero) *CloudZeroPlugin {
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
