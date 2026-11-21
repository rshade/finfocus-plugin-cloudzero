package plugin

import (
	"context"
	"sort"
	"strings"
	"time"

	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rshade/finfocus-plugin-cloudzero/internal/czclient"
	"github.com/rshade/finfocus-plugin-cloudzero/internal/mapping"
)

// GetActualCost returns one row per day inside the requested window.
// An empty list means CloudZero had no rows. It is not a zero cost.
func (p *CloudZeroPlugin) GetActualCost(
	ctx context.Context, req *pbc.GetActualCostRequest,
) (*pbc.GetActualCostResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "request cannot be nil")
	}
	start, end, err := window(req.GetStart(), req.GetEnd())
	if err != nil {
		return nil, err
	}
	if err = p.ready(); err != nil {
		return nil, err
	}
	filter, err := p.mapper.Filter(req.GetResource(), req.GetTags(), req.GetArn(), req.GetResourceId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	rows, err := p.costsFor(ctx, start, end, filter.Shared)
	if err != nil {
		mapped := statusFrom(err)
		p.logger.Warn().Err(mapped).Msg("actual cost query failed")
		return nil, mapped
	}
	return responseFor(p.rowsFor(rows, filter, start, end)), nil
}

func (p *CloudZeroPlugin) ready() error {
	if missing := p.missingSetting(); missing != "" {
		return status.Error(codes.FailedPrecondition, missing)
	}
	return nil
}

func window(start, end *timestamppb.Timestamp) (time.Time, time.Time, error) {
	if start == nil || end == nil {
		return time.Time{}, time.Time{}, status.Error(codes.InvalidArgument, "start and end are required")
	}
	from, to := start.AsTime(), end.AsTime()
	if !to.After(from) {
		return time.Time{}, time.Time{}, status.Error(codes.InvalidArgument, "end must be after start")
	}
	return from, to, nil
}

// cloudProviderDimension is grouped on, not filtered on. CloudZero's filter is
// case-sensitive and its spellings are undocumented, so each row carries the
// value and mapping.Filter compares it with the resource's provider.
const cloudProviderDimension = "CloudProvider"

// costsFor returns every resource's daily rows for the months that cover the
// window, filtered only by the shared dimensions. Resources that share those
// dimensions share one upstream call and one cache entry, including requests
// that arrive while that call is in flight.
func (p *CloudZeroPlugin) costsFor(
	ctx context.Context, start, end time.Time, shared map[string]string,
) ([]czclient.CostRow, error) {
	from, to := monthWindow(start, end)
	key := strings.Join([]string{
		from.Format(time.RFC3339), to.Format(time.RFC3339), p.cfg.CostType, filterKey(shared),
	}, "|")
	fetch := func(fetchCtx context.Context) ([]czclient.CostRow, error) {
		filters := make(map[string][]string, len(shared))
		for dimension, value := range shared {
			filters[dimension] = []string{value}
		}
		return p.client.GetCosts(fetchCtx, czclient.CostQuery{
			Start:    from,
			End:      to,
			GroupBy:  []string{p.cfg.ResourceDimension, cloudProviderDimension},
			Filters:  filters,
			CostType: p.cfg.CostType,
		})
	}
	return cachedFetch(ctx, p.cache, p.costsInFlight, key, p.ttlFor(to.Add(-time.Nanosecond)), fetch)
}

// filterKey serializes dimension filters with sorted keys, so equal filters
// give equal keys.
func filterKey(filters map[string]string) string {
	keys := make([]string, 0, len(filters))
	for key := range filters {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(filters[key])
		b.WriteByte(';')
	}
	return b.String()
}

func monthWindow(start, end time.Time) (time.Time, time.Time) {
	start, end = start.UTC(), end.UTC()
	from := time.Date(start.Year(), start.Month(), 1, 0, 0, 0, 0, time.UTC)
	last := end.Add(-time.Nanosecond)
	to := time.Date(last.Year(), last.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0)
	return from, to
}

// ttlFor returns the short TTL for the newest two days and the normal TTL for
// anything older.
func (p *CloudZeroPlugin) ttlFor(day time.Time) time.Duration {
	now := p.now().UTC()
	yesterday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, -1)
	if !day.UTC().Before(yesterday) {
		return p.cfg.RecentTTL
	}
	return p.cfg.CacheTTL
}

// rootVolumeSource is appended to the source of root volume rows, so core's
// breakdown, which is keyed by source, shows the volume on its own line.
const rootVolumeSource = " root volume"

// rowsFor slices the resource's rows out of a month query, one result per day.
// Rows for its root volume come back as separate results with their own
// source; core adds them into the resource's total.
func (p *CloudZeroPlugin) rowsFor(
	rows []czclient.CostRow,
	filter mapping.Filter,
	start, end time.Time,
) []*pbc.ActualCostResult {
	source := PluginName + ":" + p.cfg.CostType + " " + p.cfg.Currency
	out := make([]*pbc.ActualCostResult, 0, len(rows))
	for _, row := range rows {
		resource, provider := row.Dimensions[p.cfg.ResourceDimension], row.Dimensions[cloudProviderDimension]
		rowSource := source
		switch {
		case filter.Matches(resource, provider):
		case filter.MatchesRootVolume(resource, provider):
			rowSource += rootVolumeSource
		default:
			continue
		}
		if !overlaps(row.UsageDate, start, end) {
			continue
		}
		out = append(out, &pbc.ActualCostResult{
			Timestamp: timestamppb.New(row.UsageDate),
			Cost:      row.Cost,
			Source:    rowSource,
			ExpiresAt: timestamppb.New(p.now().Add(p.ttlFor(row.UsageDate))),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].GetTimestamp().AsTime().Before(out[j].GetTimestamp().AsTime())
	})
	return out
}

func overlaps(day, start, end time.Time) bool {
	day = day.UTC()
	return day.Before(end) && day.AddDate(0, 0, 1).After(start)
}

func responseFor(rows []*pbc.ActualCostResult) *pbc.GetActualCostResponse {
	hint := pbc.FallbackHint_FALLBACK_HINT_UNSPECIFIED
	if len(rows) == 0 {
		hint = pbc.FallbackHint_FALLBACK_HINT_RECOMMENDED
	}
	return &pbc.GetActualCostResponse{Results: rows, FallbackHint: hint}
}
