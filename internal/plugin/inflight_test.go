package plugin

import (
	"context"
	"fmt"
	"testing"
	"time"

	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/rshade/finfocus-plugin-cloudzero/internal/czclient"
)

type actualAnswer struct {
	resp *pbc.GetActualCostResponse
	err  error
}

func TestGetActualCostConcurrentRequestsShareOneCall(t *testing.T) {
	t.Parallel()
	const requests = 20
	rows := make([]czclient.CostRow, requests)
	for i := range requests {
		czrn := fmt.Sprintf("czrn:aws:ec2:us-east-1:123456789012:instance:i-%d", i)
		rows[i] = costRow(day(2026, 3, 15), float64(i+1), czrn)
	}
	costs := &fakeCosts{rows: rows, gate: make(chan struct{})}
	plug := gatedPlugin(costs)

	answers := make([]<-chan actualAnswer, requests)
	for i := range requests {
		arn := fmt.Sprintf("arn:aws:ec2:us-east-1:123456789012:instance/i-%d", i)
		answers[i] = askActual(context.Background(), plug, arn)
	}
	waitForWaiters(t, plug, requests)
	close(costs.gate)

	for i, answer := range answers {
		got := receive(t, answer)
		require.NoError(t, got.err)
		require.Len(t, got.resp.GetResults(), 1)
		assert.InDelta(t, float64(i+1), got.resp.GetResults()[0].GetCost(), 0)
	}
	assert.Len(t, costs.calls(), 1)
}

func TestGetActualCostWaiterHonorsItsContext(t *testing.T) {
	t.Parallel()
	costs := &fakeCosts{
		rows: []czclient.CostRow{
			costRow(day(2026, 3, 15), 1, firstCZRN),
			costRow(day(2026, 3, 15), 2, secondCZRN),
		},
		gate: make(chan struct{}),
	}
	plug := gatedPlugin(costs)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	leaving := askActual(ctx, plug, firstARN)
	waitForWaiters(t, plug, 1)
	staying := askActual(context.Background(), plug, secondARN)
	waitForWaiters(t, plug, 2)

	cancel()
	assert.Equal(t, codes.Canceled, status.Code(receive(t, leaving).err))
	assert.Equal(t, 1, plug.fetches.waiting())

	close(costs.gate)
	got := receive(t, staying)
	require.NoError(t, got.err)
	require.Len(t, got.resp.GetResults(), 1)
	assert.InDelta(t, 2.0, got.resp.GetResults()[0].GetCost(), 0)
	assert.Len(t, costs.calls(), 1)
}

func TestGetActualCostSharedFailureIsNotCached(t *testing.T) {
	t.Parallel()
	costs := &fakeCosts{
		err:  &czclient.APIError{Kind: czclient.KindRateLimit, Status: 429},
		gate: make(chan struct{}),
	}
	plug := gatedPlugin(costs)

	arns := []string{firstARN, secondARN, firstARN}
	answers := make([]<-chan actualAnswer, len(arns))
	for i, arn := range arns {
		answers[i] = askActual(context.Background(), plug, arn)
	}
	waitForWaiters(t, plug, len(arns))
	close(costs.gate)
	for _, answer := range answers {
		assert.Equal(t, codes.ResourceExhausted, status.Code(receive(t, answer).err))
	}
	require.Len(t, costs.calls(), 1)

	_, err := plug.GetActualCost(context.Background(), actualReq(day(2026, 3, 1), day(2026, 3, 31), firstARN))
	assert.Equal(t, codes.ResourceExhausted, status.Code(err))
	assert.Len(t, costs.calls(), 2)
}

func TestGetActualCostCanceledCallerStartsNoFetch(t *testing.T) {
	t.Parallel()
	costs := &fakeCosts{rows: []czclient.CostRow{costRow(day(2026, 3, 15), 1, firstCZRN)}}
	plug := gatedPlugin(costs)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := plug.GetActualCost(ctx, actualReq(day(2026, 3, 1), day(2026, 3, 31), firstARN))
	assert.Equal(t, codes.Canceled, status.Code(err))
	assert.Empty(t, costs.calls())
}

func TestDetachedKeepsDeadlineNotCancel(t *testing.T) {
	t.Parallel()
	deadline := time.Now().Add(time.Hour)
	parent, cancel := context.WithDeadline(context.Background(), deadline)
	fetchCtx, release := detached(parent)
	defer release()
	cancel()

	require.NoError(t, fetchCtx.Err())
	got, ok := fetchCtx.Deadline()
	require.True(t, ok)
	assert.Equal(t, deadline, got)
}

func gatedPlugin(costs *fakeCosts) *CloudZeroPlugin {
	plug := fakePlugin(readyConfig(), costs)
	plug.now = func() time.Time { return day(2026, 4, 1) }
	return plug
}

func askActual(ctx context.Context, plug *CloudZeroPlugin, arn string) <-chan actualAnswer {
	answer := make(chan actualAnswer, 1)
	go func() {
		resp, err := plug.GetActualCost(ctx, actualReq(day(2026, 3, 1), day(2026, 3, 31), arn))
		answer <- actualAnswer{resp: resp, err: err}
	}()
	return answer
}

func waitForWaiters(t *testing.T, plug *CloudZeroPlugin, want int) {
	t.Helper()
	require.Eventually(t, func() bool { return plug.fetches.waiting() == want }, 5*time.Second, time.Millisecond)
}

func receive[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(5 * time.Second):
		require.FailNow(t, "no answer within 5s")
		var zero T
		return zero
	}
}
