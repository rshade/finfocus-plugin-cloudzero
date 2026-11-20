package plugin

import (
	"context"
	"math"
	"strings"
	"time"

	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/rshade/finfocus-plugin-cloudzero/internal/czclient"
)

const (
	budgetsCacheKey = "budgets"
	monthlyBudget   = "monthly"
	viewIDMetadata  = "cloudzero_view_id"
)

// GetBudgets lists CloudZero's monthly budgets with the current month's
// planned amount as the limit and CloudZero's actual as current spend. The
// status is always set, because core sends no include_status and computes
// health only from a status. Core derives percentages, forecast, and health.
// The request filter is ignored: core filters the budgets it gets back.
func (p *CloudZeroPlugin) GetBudgets(ctx context.Context, _ *pbc.GetBudgetsRequest) (*pbc.GetBudgetsResponse, error) {
	if !p.cfg.HasAPIKey() {
		return nil, status.Error(codes.FailedPrecondition, keyNotSet)
	}
	raw, err := p.cloudZeroBudgets(ctx)
	if err != nil {
		mapped := statusFrom(err)
		p.logger.Warn().Err(mapped).Msg("budgets query failed")
		return nil, mapped
	}
	now := p.now().UTC()
	month := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
	budgets := make([]*pbc.Budget, 0, len(raw))
	var notMonthly, unplanned int
	for _, b := range raw {
		budget, outcome := p.budgetFor(b, month)
		switch outcome {
		case keptBudget:
			budgets = append(budgets, budget)
		case skippedNotMonthly:
			notMonthly++
		case skippedUnplanned:
			unplanned++
		}
	}
	if notMonthly+unplanned > 0 {
		p.logger.Debug().
			Int("not_monthly", notMonthly).
			Int("nothing_planned_this_month", unplanned).
			Msg("cloudzero budgets left out")
	}
	return &pbc.GetBudgetsResponse{
		Budgets: budgets,
		Summary: &pbc.BudgetSummary{
			TotalBudgets: int32(min(len(budgets), math.MaxInt32)),
		},
	}, nil
}

// cloudZeroBudgets reads every budget once per RecentTTL. Concurrent callers
// share one fetch, and a failed fetch is not cached.
func (p *CloudZeroPlugin) cloudZeroBudgets(ctx context.Context) ([]czclient.Budget, error) {
	return cachedFetch(ctx, p.cache, p.budgetsInFlight, budgetsCacheKey, p.cfg.RecentTTL, p.client.GetBudgets)
}

// budgetOutcome says whether budgetFor kept a budget, or why it left it out.
type budgetOutcome int

const (
	keptBudget budgetOutcome = iota
	skippedNotMonthly
	skippedUnplanned
)

// budgetFor maps a CloudZero budget, or says why it is left out: a
// granularity other than monthly, or nothing above zero planned for month. A
// budget without a granularity counts as monthly, the only value the docs show.
func (p *CloudZeroPlugin) budgetFor(b czclient.Budget, month time.Time) (*pbc.Budget, budgetOutcome) {
	granularity := strings.TrimSpace(b.Granularity)
	if granularity != "" && !strings.EqualFold(granularity, monthlyBudget) {
		return nil, skippedNotMonthly
	}
	limit, ok := b.Monthly[month]
	if !ok || limit <= 0 {
		return nil, skippedUnplanned
	}
	budget := &pbc.Budget{
		Id:         b.ID,
		Name:       b.Name,
		Source:     pluginName,
		Amount:     &pbc.BudgetAmount{Limit: limit, Currency: p.cfg.Currency},
		Period:     pbc.BudgetPeriod_BUDGET_PERIOD_MONTHLY,
		Thresholds: thresholdsFor(b.Alerts),
	}
	if b.ViewID != "" {
		budget.Metadata = map[string]string{viewIDMetadata: b.ViewID}
	}
	if b.Actual != nil {
		budget.Status = &pbc.BudgetStatus{CurrentSpend: *b.Actual, Currency: p.cfg.Currency}
	}
	return budget, keptBudget
}

func thresholdsFor(alerts []float64) []*pbc.BudgetThreshold {
	if len(alerts) == 0 {
		return nil
	}
	thresholds := make([]*pbc.BudgetThreshold, 0, len(alerts))
	for _, percent := range alerts {
		thresholds = append(thresholds, &pbc.BudgetThreshold{
			Percentage: percent,
			Type:       pbc.ThresholdType_THRESHOLD_TYPE_ACTUAL,
		})
	}
	return thresholds
}
