package plugin

import (
	"context"
	"testing"
	"time"

	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rshade/finfocus-plugin-cloudzero/internal/config"
	"github.com/rshade/finfocus-plugin-cloudzero/internal/czclient"
)

func actualBatch(resources ...*pbc.ResourceDescriptor) *pbc.BatchCostRequest {
	return &pbc.BatchCostRequest{
		QueryType: pbc.CostQueryType_COST_QUERY_TYPE_ACTUAL,
		Start:     timestamppb.New(day(2026, 3, 1)),
		End:       timestamppb.New(day(2026, 3, 20)),
		Resources: resources,
	}
}

func TestBatchCostOneCallKeepsOrderAndPartialFailure(t *testing.T) {
	t.Parallel()
	costs := &fakeCloudZero{rows: []czclient.CostRow{
		costRow(day(2026, 3, 15), 1, firstCZRN),
		costRow(day(2026, 3, 15), 2, secondCZRN),
		costRow(day(2026, 3, 15), 9, "other"),
	}}
	plug := fakePlugin(readyConfig(), costs)
	missing := &pbc.ResourceDescriptor{Provider: "aws", Region: "us-east-1"}
	resp, err := plug.BatchCost(context.Background(), actualBatch(
		resource(firstARN), resource(secondARN), resource(firstARN), missing,
	))
	require.NoError(t, err)
	require.Len(t, resp.GetResults(), 4)
	costOf := func(i int) float64 {
		results := resp.GetResults()[i].GetCostData().GetActualCost().GetResults()
		require.Len(t, results, 1)
		return results[0].GetCost()
	}
	assert.InDelta(t, 1.0, costOf(0), 0)
	assert.InDelta(t, 2.0, costOf(1), 0)
	assert.InDelta(t, 1.0, costOf(2), 0)
	assert.Equal(t, int32(codes.InvalidArgument), resp.GetResults()[3].GetError().GetCode())
	assert.Len(t, costs.calls(), 1)
}

func TestBatchCostUpstreamFailure(t *testing.T) {
	t.Parallel()
	costs := &fakeCloudZero{err: &czclient.APIError{Kind: czclient.KindBadRequest, Status: 400}}
	resp, err := fakePlugin(readyConfig(), costs).BatchCost(context.Background(), actualBatch(resource(firstARN)))
	require.NoError(t, err)
	assert.Equal(t, int32(codes.InvalidArgument), resp.GetResults()[0].GetError().GetCode())
}

func TestBatchCostNotReady(t *testing.T) {
	t.Parallel()
	costs := &fakeCloudZero{}
	resp, err := fakePlugin(config.Config{}, costs).BatchCost(
		context.Background(), actualBatch(resource(firstARN), resource(secondARN)),
	)
	require.NoError(t, err)
	require.Len(t, resp.GetResults(), 2)
	for _, result := range resp.GetResults() {
		assert.Equal(t, int32(codes.FailedPrecondition), result.GetError().GetCode())
	}
	assert.Empty(t, costs.calls())
}

func TestBatchCostProjectedEmptyAndBadWindow(t *testing.T) {
	t.Parallel()
	plug := fakePlugin(readyConfig(), &fakeCloudZero{})
	empty, err := plug.BatchCost(context.Background(), &pbc.BatchCostRequest{})
	require.NoError(t, err)
	assert.Empty(t, empty.GetResults())

	nilResp, err := plug.BatchCost(context.Background(), nil)
	require.NoError(t, err)
	assert.Empty(t, nilResp.GetResults())

	resp, err := plug.BatchCost(context.Background(), &pbc.BatchCostRequest{
		QueryType: pbc.CostQueryType_COST_QUERY_TYPE_PROJECTED,
		Resources: []*pbc.ResourceDescriptor{resource(firstARN)},
	})
	require.NoError(t, err)
	require.Len(t, resp.GetResults(), 1)
	assert.Equal(t, int32(codes.Unimplemented), resp.GetResults()[0].GetError().GetCode())

	now := time.Now()
	_, err = plug.BatchCost(context.Background(), &pbc.BatchCostRequest{
		QueryType: pbc.CostQueryType_COST_QUERY_TYPE_ACTUAL,
		Start:     timestamppb.New(now), End: timestamppb.New(now),
		Resources: []*pbc.ResourceDescriptor{resource(firstARN)},
	})
	require.Error(t, err)
}
