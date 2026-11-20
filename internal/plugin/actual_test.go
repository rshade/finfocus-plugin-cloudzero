package plugin

import (
	"bytes"
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/rs/zerolog"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rshade/finfocus-plugin-cloudzero/internal/config"
	"github.com/rshade/finfocus-plugin-cloudzero/internal/czclient"
)

func TestGetActualCostSlicesMonthAndCaches(t *testing.T) {
	t.Parallel()
	costs := &fakeCloudZero{rows: []czclient.CostRow{
		costRow(day(2026, 3, 1), 1, firstCZRN),
		costRow(day(2026, 3, 15), 2, firstCZRN),
		costRow(day(2026, 3, 20), 9, firstCZRN),
	}}
	plug := fakePlugin(readyConfig(), costs)
	plug.now = func() time.Time { return day(2026, 4, 1) }
	req := actualReq(day(2026, 3, 10), day(2026, 3, 16), firstARN)

	resp, err := plug.GetActualCost(context.Background(), req)
	require.NoError(t, err)
	require.Len(t, resp.GetResults(), 1)
	assert.InDelta(t, 2.0, resp.GetResults()[0].GetCost(), 0)
	assert.Equal(t, "cloudzero:real_cost USD", resp.GetResults()[0].GetSource())
	assert.NotNil(t, resp.GetResults()[0].GetExpiresAt())

	_, err = plug.GetActualCost(context.Background(), req)
	require.NoError(t, err)
	queries := costs.calls()
	require.Len(t, queries, 1)
	assert.Equal(t, day(2026, 3, 1), queries[0].Start)
	assert.Equal(t, day(2026, 4, 1), queries[0].End)
	assert.Equal(t, []string{"Resource", "CloudProvider"}, queries[0].GroupBy)
}

func TestGetActualCostSharesOneCallAcrossResources(t *testing.T) {
	t.Parallel()
	costs := &fakeCloudZero{rows: []czclient.CostRow{
		costRow(day(2026, 3, 15), 1, firstCZRN),
		costRow(day(2026, 3, 15), 2, secondCZRN),
		costRow(day(2026, 3, 15), 9, "czrn:aws:ec2:us-east-1:123456789012:instance:other"),
	}}
	plug := fakePlugin(readyConfig(), costs)
	plug.now = func() time.Time { return day(2026, 4, 1) }

	first, err := plug.GetActualCost(context.Background(), actualReq(day(2026, 3, 10), day(2026, 3, 20), firstARN))
	require.NoError(t, err)
	second, err := plug.GetActualCost(context.Background(), actualReq(day(2026, 3, 12), day(2026, 3, 20), secondARN))
	require.NoError(t, err)

	require.Len(t, first.GetResults(), 1)
	assert.InDelta(t, 1.0, first.GetResults()[0].GetCost(), 0)
	require.Len(t, second.GetResults(), 1)
	assert.InDelta(t, 2.0, second.GetResults()[0].GetCost(), 0)
	queries := costs.calls()
	require.Len(t, queries, 1)
	assert.Equal(t, map[string][]string{
		"Account": {"123456789012"}, "Region": {"us-east-1"},
	}, queries[0].Filters)
}

func TestGetActualCostMatchesResourceWithoutARN(t *testing.T) {
	t.Parallel()
	costs := &fakeCloudZero{rows: []czclient.CostRow{
		costRow(day(2026, 11, 1), 1.07, "czrn:aws:ec2:us-west-2:123456789012:nat-gateway:nat-0123456789abcdef0"),
		costRow(day(2026, 11, 1), 0.73, "czrn:aws:ec2:us-west-2:123456789012:nat-gateway:nat-0fedcba9876543210"),
		costRow(day(2026, 11, 1), 0.33, "czrn:aws:ec2:us-west-2:123456789012:instance:i-0123456789abcdef1"),
	}}
	plug := fakePlugin(readyConfig(), costs)
	plug.now = func() time.Time { return day(2026, 11, 18) }
	resp, err := plug.GetActualCost(context.Background(), &pbc.GetActualCostRequest{
		Start:      timestamppb.New(day(2026, 11, 1)),
		End:        timestamppb.New(day(2026, 11, 2)),
		ResourceId: "nat-0123456789abcdef0",
		Resource:   &pbc.ResourceDescriptor{Provider: "aws", Region: "us-west-2"},
	})
	require.NoError(t, err)
	require.Len(t, resp.GetResults(), 1)
	assert.InDelta(t, 1.07, resp.GetResults()[0].GetCost(), 0)
	queries := costs.calls()
	require.Len(t, queries, 1)
	assert.Equal(t, map[string][]string{"Region": {"us-west-2"}}, queries[0].Filters)
	assert.Equal(t, []string{"Resource", "CloudProvider"}, queries[0].GroupBy)
}

func TestGetActualCostEmptyIsNoData(t *testing.T) {
	t.Parallel()
	plug := fakePlugin(readyConfig(), &fakeCloudZero{})
	now := time.Now()
	resp, err := plug.GetActualCost(context.Background(), actualReq(now.Add(-48*time.Hour), now, firstARN))
	require.NoError(t, err)
	assert.Empty(t, resp.GetResults())
	assert.Equal(t, pbc.FallbackHint_FALLBACK_HINT_RECOMMENDED, resp.GetFallbackHint())
}

func TestGetActualCostRecentDayUsesShortTTL(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 20, 12, 0, 0, 0, time.UTC)
	plug := fakePlugin(readyConfig(), &fakeCloudZero{rows: []czclient.CostRow{
		costRow(day(2026, 3, 1), 3, firstCZRN),
		costRow(day(2026, 3, 20), 4, firstCZRN),
	}})
	plug.now = func() time.Time { return now }
	resp, err := plug.GetActualCost(context.Background(), actualReq(day(2026, 3, 1), now.Add(time.Hour), firstARN))
	require.NoError(t, err)
	require.Len(t, resp.GetResults(), 2)
	assert.Equal(t, now.Add(plug.cfg.CacheTTL), resp.GetResults()[0].GetExpiresAt().AsTime())
	assert.Equal(t, now.Add(plug.cfg.RecentTTL), resp.GetResults()[1].GetExpiresAt().AsTime())
}

func TestGetActualCostRejectsBadRequests(t *testing.T) {
	t.Parallel()
	now := time.Now()
	noDimension := readyConfig()
	noDimension.ResourceDimension = ""
	tests := []struct {
		name string
		cfg  config.Config
		req  *pbc.GetActualCostRequest
		want codes.Code
	}{
		{"nil request", readyConfig(), nil, codes.InvalidArgument},
		{"no window", readyConfig(), &pbc.GetActualCostRequest{}, codes.InvalidArgument},
		{
			"empty window", readyConfig(),
			&pbc.GetActualCostRequest{Start: timestamppb.New(now), End: timestamppb.New(now)},
			codes.InvalidArgument,
		},
		{
			"no identity", readyConfig(),
			&pbc.GetActualCostRequest{
				Start: timestamppb.New(now.Add(-time.Hour)), End: timestamppb.New(now),
				Resource: &pbc.ResourceDescriptor{Provider: "aws"},
			},
			codes.InvalidArgument,
		},
		{
			"no key", config.Config{ResourceDimension: "Resource"},
			actualReq(now.Add(-time.Hour), now, firstARN), codes.FailedPrecondition,
		},
		{"no dimension", noDimension, actualReq(now.Add(-time.Hour), now, firstARN), codes.FailedPrecondition},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			costs := &fakeCloudZero{}
			_, err := fakePlugin(tt.cfg, costs).GetActualCost(context.Background(), tt.req)
			assert.Equal(t, tt.want, status.Code(err))
			assert.Empty(t, costs.calls())
		})
	}
}

func TestGetActualCostRateLimitAndBudget(t *testing.T) {
	t.Parallel()
	limited := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"slow","type":"Rate"}}`))
	}
	now := time.Now()
	plug := httpPlugin(t, readyConfig(), limited, zerolog.Nop())
	_, err := plug.GetActualCost(context.Background(), actualReq(now.Add(-time.Hour), now, firstARN))
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	assert.NotContains(t, status.Convert(err).Message(), "slow")

	noBudget := readyConfig()
	noBudget.DailyBudget = 0
	plug = httpPlugin(t, noBudget, limited, zerolog.Nop())
	_, err = plug.GetActualCost(context.Background(), actualReq(now.Add(-time.Hour), now, firstARN))
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), "budget")
}

func TestGetActualCostRateLimitNamesWait(t *testing.T) {
	t.Parallel()
	limited := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"Please wait 24 minute(s) before trying again.","type":"x"}}`))
	}
	now := time.Now()
	plug := httpPlugin(t, readyConfig(), limited, zerolog.Nop())
	_, err := plug.GetActualCost(context.Background(), actualReq(now.Add(-time.Hour), now, firstARN))
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	msg := status.Convert(err).Message()
	assert.Contains(t, msg, "retry in 24 minutes")
	assert.NotContains(t, msg, "Please wait")
}

func TestKeyNeverLogged(t *testing.T) {
	t.Parallel()
	const key = "cz-test-key-not-for-logs"
	var buf bytes.Buffer
	cfg := readyConfig()
	cfg.APIKey = key
	denied := func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"denied"}`))
	}
	plug := httpPlugin(t, cfg, denied, zerolog.New(&buf))
	_, err := plug.GetActualCost(context.Background(), actualReq(time.Now().Add(-time.Hour), time.Now(), firstARN))
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
	assert.NotContains(t, err.Error(), key)
	assert.NotContains(t, buf.String(), key)
}

func TestGetActualCostIncludesRootVolume(t *testing.T) {
	t.Parallel()
	const (
		rootCZRN  = "czrn:aws:ec2:us-east-1:123456789012:volume:vol-1"
		otherCZRN = "czrn:aws:ec2:us-east-1:123456789012:volume:vol-2"
	)
	costs := &fakeCloudZero{rows: []czclient.CostRow{
		costRow(day(2026, 3, 1), 6, firstCZRN),
		costRow(day(2026, 3, 2), 4, firstCZRN),
		costRow(day(2026, 3, 1), 0.5, rootCZRN),
		costRow(day(2026, 3, 2), 0.3, rootCZRN),
		costRow(day(2026, 3, 1), 9, otherCZRN),
		costRow(day(2026, 3, 1), 7, secondCZRN),
	}}
	plug := fakePlugin(readyConfig(), costs)
	attrs, err := structpb.NewStruct(map[string]any{"rootBlockDevice": map[string]any{"volumeId": "vol-1"}})
	require.NoError(t, err)
	req := actualReq(day(2026, 3, 1), day(2026, 3, 31), firstARN)
	req.Resource.Attributes = attrs

	resp, err := plug.GetActualCost(context.Background(), req)
	require.NoError(t, err)

	bySource := map[string]float64{}
	for _, row := range resp.GetResults() {
		bySource[row.GetSource()] += row.GetCost()
	}
	assert.InDelta(t, 10.0, bySource["cloudzero:real_cost USD"], 1e-9)
	assert.InDelta(t, 0.8, bySource["cloudzero:real_cost USD root volume"], 1e-9)
	assert.Len(t, bySource, 2, "another instance's volume is not added")
	assert.Len(t, resp.GetResults(), 4, "one result per day per source")
	assert.Len(t, costs.calls(), 1, "the volume comes from the same query")
}
