package plugin

import (
	"bytes"
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/rshade/finfocus-plugin-cloudzero/internal/config"
	"github.com/rshade/finfocus-plugin-cloudzero/internal/czclient"
)

var (
	budgetMonth = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	budgetNow   = func() time.Time { return time.Date(2026, 11, 19, 15, 0, 0, 0, time.UTC) }
)

func budgetPlugin(costs *fakeCloudZero) *CloudZeroPlugin {
	plug := fakePlugin(readyConfig(), costs)
	plug.now = budgetNow
	return plug
}

func monthlyBudgetOf(id string, limit float64, actual *float64) czclient.Budget {
	return czclient.Budget{
		ID: id, Name: "budget " + id, ViewID: "", Granularity: "monthly",
		Monthly: map[time.Time]float64{budgetMonth: limit}, Alerts: nil, Planned: nil, Actual: actual,
	}
}

func TestGetBudgetsMapsCloudZeroBudgets(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile(filepath.Join("..", "czclient", "testdata", "budgets_docs.json"))
	require.NoError(t, err)
	plug := httpPlugin(t, readyConfig(), func(w http.ResponseWriter, _ *http.Request) {
		_, writeErr := w.Write(body)
		assert.NoError(t, writeErr)
	}, zerolog.Nop())
	plug.now = budgetNow

	resp, err := plug.GetBudgets(context.Background(), &pbc.GetBudgetsRequest{})
	require.NoError(t, err)
	require.Len(t, resp.GetBudgets(), 1, "the quarterly budget is left out")
	got := resp.GetBudgets()[0]
	assert.Equal(t, "00000000-0000-0000-0000-00000000b001", got.GetId())
	assert.Equal(t, "Platform monthly", got.GetName())
	assert.Equal(t, "cloudzero", got.GetSource())
	assert.InDelta(t, 500.0, got.GetAmount().GetLimit(), 1e-9)
	assert.Equal(t, "USD", got.GetAmount().GetCurrency())
	assert.Equal(t, pbc.BudgetPeriod_BUDGET_PERIOD_MONTHLY, got.GetPeriod())
	assert.Equal(t, map[string]string{"cloudzero_view_id": "00000000-0000-0000-0000-00000000f001"}, got.GetMetadata())
	assert.Nil(t, got.GetFilter())
}

func TestGetBudgetsSkipsUnplannedMonth(t *testing.T) {
	t.Parallel()
	october := czclient.Budget{
		ID: "old", Name: "old", ViewID: "", Granularity: "monthly",
		Monthly: map[time.Time]float64{budgetMonth.AddDate(0, -1, 0): 100}, Alerts: nil, Planned: nil, Actual: nil,
	}
	quarterly := monthlyBudgetOf("quarterly", 900, nil)
	quarterly.Granularity = "quarterly"
	unlabelled := monthlyBudgetOf("unlabelled", 300, nil)
	unlabelled.Granularity = ""
	costs := &fakeCloudZero{budgets: []czclient.Budget{
		october, quarterly, monthlyBudgetOf("zero", 0, nil), monthlyBudgetOf("kept", 250, nil), unlabelled,
	}}
	var logs bytes.Buffer
	plug := budgetPlugin(costs)
	plug.logger = zerolog.New(&logs).Level(zerolog.DebugLevel)

	resp, err := plug.GetBudgets(context.Background(), &pbc.GetBudgetsRequest{})
	require.NoError(t, err)
	limits := make(map[string]float64, len(resp.GetBudgets()))
	for _, budget := range resp.GetBudgets() {
		limits[budget.GetId()] = budget.GetAmount().GetLimit()
	}
	assert.Equal(t, map[string]float64{"kept": 250, "unlabelled": 300}, limits,
		"a budget without a granularity counts as monthly")
	assert.Contains(t, logs.String(), `"not_monthly":1`)
	assert.Contains(t, logs.String(), `"nothing_planned_this_month":2`)
}

func TestGetBudgetsStatusIsSpendOnly(t *testing.T) {
	t.Parallel()
	spent := 120.5
	costs := &fakeCloudZero{budgets: []czclient.Budget{
		monthlyBudgetOf("spent", 500, &spent), monthlyBudgetOf("unknown", 500, nil),
	}}
	resp, err := budgetPlugin(costs).GetBudgets(context.Background(), &pbc.GetBudgetsRequest{})
	require.NoError(t, err)
	require.Len(t, resp.GetBudgets(), 2)

	st := resp.GetBudgets()[0].GetStatus()
	require.NotNil(t, st, "status is set although include_status is false")
	assert.InDelta(t, 120.5, st.GetCurrentSpend(), 1e-9)
	assert.Equal(t, "USD", st.GetCurrency())
	assert.Zero(t, st.GetForecastedSpend())
	assert.Zero(t, st.GetPercentageUsed())
	assert.Zero(t, st.GetPercentageForecasted())
	assert.Equal(t, pbc.BudgetHealthStatus_BUDGET_HEALTH_STATUS_UNSPECIFIED, st.GetHealth())

	assert.Nil(t, resp.GetBudgets()[1].GetStatus(), "a missing actual leaves no status")
}

func TestGetBudgetsThresholds(t *testing.T) {
	t.Parallel()
	plug := httpPlugin(t, readyConfig(), func(w http.ResponseWriter, _ *http.Request) {
		_, err := w.Write([]byte(`{"budgets":[{"id":"b","name":"n","granularity":"monthly",
			"planned_limits":{"monthly":{"2026-11-01":{"amount":"100"}}},
			"alerts":{"80":"Y","100":"N","50":true,"0":"Y","150":"Y","high":"Y"}}]}`))
		assert.NoError(t, err)
	}, zerolog.Nop())
	plug.now = budgetNow

	resp, err := plug.GetBudgets(context.Background(), &pbc.GetBudgetsRequest{})
	require.NoError(t, err)
	require.Len(t, resp.GetBudgets(), 1)
	thresholds := resp.GetBudgets()[0].GetThresholds()
	require.Len(t, thresholds, 2, "disabled and out-of-range alerts are left out")
	for i, want := range []float64{50, 80} {
		assert.InDelta(t, want, thresholds[i].GetPercentage(), 1e-9)
		assert.Equal(t, pbc.ThresholdType_THRESHOLD_TYPE_ACTUAL, thresholds[i].GetType())
		assert.False(t, thresholds[i].GetTriggered())
	}
}

func TestGetBudgetsSummary(t *testing.T) {
	t.Parallel()
	resp, err := budgetPlugin(&fakeCloudZero{}).GetBudgets(context.Background(), &pbc.GetBudgetsRequest{})
	require.NoError(t, err)
	assert.Empty(t, resp.GetBudgets())
	require.NotNil(t, resp.GetSummary())
	assert.Zero(t, resp.GetSummary().GetTotalBudgets())

	two := &fakeCloudZero{budgets: []czclient.Budget{monthlyBudgetOf("a", 1, nil), monthlyBudgetOf("b", 2, nil)}}
	resp, err = budgetPlugin(two).GetBudgets(context.Background(), &pbc.GetBudgetsRequest{})
	require.NoError(t, err)
	summary := resp.GetSummary()
	assert.Equal(t, int32(2), summary.GetTotalBudgets())
	assert.Zero(t, summary.GetBudgetsOk()+summary.GetBudgetsWarning()+
		summary.GetBudgetsCritical()+summary.GetBudgetsExceeded())
}

func TestGetBudgetsCachesOneFetch(t *testing.T) {
	t.Parallel()

	t.Run("sequential calls inside the window", func(t *testing.T) {
		t.Parallel()
		costs := &fakeCloudZero{budgets: []czclient.Budget{monthlyBudgetOf("a", 1, nil)}}
		plug := budgetPlugin(costs)
		for range 2 {
			_, err := plug.GetBudgets(context.Background(), &pbc.GetBudgetsRequest{})
			require.NoError(t, err)
		}
		assert.Equal(t, 1, costs.budgetFetches())
	})

	t.Run("concurrent calls share one fetch", func(t *testing.T) {
		t.Parallel()
		gate := make(chan struct{})
		costs := &fakeCloudZero{budgets: []czclient.Budget{monthlyBudgetOf("a", 1, nil)}, gate: gate}
		plug := budgetPlugin(costs)
		var wg sync.WaitGroup
		for range 3 {
			wg.Go(func() {
				resp, err := plug.GetBudgets(context.Background(), &pbc.GetBudgetsRequest{})
				assert.NoError(t, err)
				assert.Len(t, resp.GetBudgets(), 1)
			})
		}
		require.Eventually(t, func() bool {
			return plug.budgetsInFlight.waiting() == 3
		}, 5*time.Second, time.Millisecond)
		close(gate)
		wg.Wait()
		assert.Equal(t, 1, costs.budgetFetches())
	})
}

func TestGetBudgetsErrors(t *testing.T) {
	t.Parallel()

	t.Run("missing key makes no call", func(t *testing.T) {
		t.Parallel()
		costs := &fakeCloudZero{}
		_, err := fakePlugin(config.Config{}, costs).GetBudgets(context.Background(), &pbc.GetBudgetsRequest{})
		assert.Equal(t, codes.FailedPrecondition, status.Code(err))
		assert.Zero(t, costs.budgetFetches())
	})

	t.Run("no resource dimension needed", func(t *testing.T) {
		t.Parallel()
		_, err := fakePlugin(config.Config{APIKey: "k", Currency: "USD"}, &fakeCloudZero{}).
			GetBudgets(context.Background(), &pbc.GetBudgetsRequest{})
		require.NoError(t, err)
	})

	tests := []struct {
		name string
		kind czclient.Kind
		want codes.Code
	}{
		{"rate limit", czclient.KindRateLimit, codes.ResourceExhausted},
		{"auth", czclient.KindAuth, codes.PermissionDenied},
		{"malformed", czclient.KindMalformed, codes.Unavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			costs := &fakeCloudZero{budgetErr: &czclient.APIError{Kind: tt.kind}}
			plug := budgetPlugin(costs)
			for range 2 {
				_, err := plug.GetBudgets(context.Background(), &pbc.GetBudgetsRequest{})
				assert.Equal(t, tt.want, status.Code(err))
			}
			assert.Equal(t, 2, costs.budgetFetches(), "a failure is not cached")
		})
	}
}
