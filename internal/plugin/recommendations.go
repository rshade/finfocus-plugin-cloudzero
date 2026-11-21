package plugin

import (
	"context"
	"strings"

	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/rshade/finfocus-plugin-cloudzero/internal/czclient"
	"github.com/rshade/finfocus-plugin-cloudzero/internal/mapping"
)

const (
	recommendationsCacheKey = "recommendations"
	dailyPeriod             = "daily"
	monthlyPeriod           = "monthly"
	annualPeriod            = "annual"
	ignoredStatus           = "ignored"
	addressedStatus         = "addressed"
	daysPerMonth            = 30
	daysPerYear             = 365
)

// GetRecommendations returns CloudZero's recommendations for the target
// resources, each under the target's id, or every recommendation under its
// CZRN when there are no targets. ignored counts as dismissed. The SDK
// helpers then apply visibility, core's filter, paging, and the summary.
func (p *CloudZeroPlugin) GetRecommendations(
	ctx context.Context, req *pbc.GetRecommendationsRequest,
) (*pbc.GetRecommendationsResponse, error) {
	if !p.cfg.HasAPIKey() {
		return nil, status.Error(codes.FailedPrecondition, keyNotSet)
	}
	period := req.GetProjectionPeriod()
	if period == "" {
		period = monthlyPeriod
	}
	scale, ok := savingsScale(period)
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "projection_period must be daily, monthly, or annual")
	}
	targets := p.recommendationTargets(req.GetTargetResources())
	var recs []*pbc.Recommendation
	var dismissed []string
	if len(req.GetTargetResources()) == 0 || len(targets) > 0 {
		raw, err := p.cloudZeroRecommendations(ctx)
		if err != nil {
			mapped := statusFrom(err)
			p.logger.Warn().Err(mapped).Msg("recommendations query failed")
			return nil, mapped
		}
		recs, dismissed = p.scopeRecommendations(raw, targets, len(req.GetTargetResources()) > 0, period, scale)
	}
	recs = pluginsdk.ApplyRecommendationVisibility(recs, pluginsdk.RecommendationVisibility{
		DismissedIDs:     dismissed,
		IncludeDismissed: req.GetIncludeDismissed(),
		ExcludedIDs:      req.GetExcludedRecommendationIds(),
	})
	recs = pluginsdk.ApplyRecommendationFilter(recs, req.GetFilter())
	page, next, err := pluginsdk.PaginateRecommendations(recs, req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid page_token")
	}
	return &pbc.GetRecommendationsResponse{
		Recommendations: page,
		Summary:         pluginsdk.CalculateRecommendationSummary(page, period),
		NextPageToken:   next,
	}, nil
}

// cloudZeroRecommendations reads every recommendation once per RecentTTL.
// Concurrent callers and later pages share one fetch. A failed fetch is not
// cached.
func (p *CloudZeroPlugin) cloudZeroRecommendations(ctx context.Context) ([]czclient.Recommendation, error) {
	return cachedFetch(
		ctx,
		p.cache,
		p.recsInFlight,
		recommendationsCacheKey,
		p.cfg.RecentTTL,
		p.client.GetRecommendations,
	)
}

// recommendationTarget is a stack resource core asked about, with the identity
// that matches CloudZero's CZRN for it.
type recommendationTarget struct {
	id       string
	identity mapping.Filter
}

func (p *CloudZeroPlugin) recommendationTargets(resources []*pbc.ResourceDescriptor) []recommendationTarget {
	targets := make([]recommendationTarget, 0, len(resources))
	for _, resource := range resources {
		identity, err := p.mapper.Identity(resource)
		if err != nil {
			continue
		}
		targets = append(targets, recommendationTarget{id: resource.GetId(), identity: identity})
	}
	if skipped := len(resources) - len(targets); skipped > 0 {
		p.logger.Debug().Int("targets_without_czrn", skipped).Msg("recommendation targets skipped")
	}
	return targets
}

// scopeRecommendations maps each recommendation once per matching target,
// or once under its CZRN when the request had no targets, and lists the ids
// CloudZero marks ignored. addressed recommendations are dropped.
func (p *CloudZeroPlugin) scopeRecommendations(
	raw []czclient.Recommendation, targets []recommendationTarget, targeted bool, period string, scale float64,
) ([]*pbc.Recommendation, []string) {
	recs := make([]*pbc.Recommendation, 0, len(raw))
	var dismissed []string
	for _, rec := range raw {
		if rec.Status == addressedStatus {
			continue
		}
		before := len(recs)
		if !targeted {
			recs = append(recs, p.recommendationFor(rec, rec.Resource, period, scale))
		}
		for _, target := range targets {
			if matchesTarget(target.identity, rec) {
				recs = append(recs, p.recommendationFor(rec, targetID(target, rec), period, scale))
			}
		}
		if len(recs) > before && rec.Status == ignoredStatus {
			dismissed = append(dismissed, rec.ID)
		}
	}
	return recs, dismissed
}

// matchesTarget reports whether a recommendation names the target itself or,
// for an AWS instance whose attributes name it, the target's root volume.
func matchesTarget(identity mapping.Filter, rec czclient.Recommendation) bool {
	for _, provider := range rec.CloudProviders {
		if identity.Matches(rec.Resource, provider) || identity.MatchesRootVolume(rec.Resource, provider) {
			return true
		}
	}
	return false
}

func targetID(target recommendationTarget, rec czclient.Recommendation) string {
	if target.id != "" {
		return target.id
	}
	return rec.Resource
}

func (p *CloudZeroPlugin) recommendationFor(
	rec czclient.Recommendation, resourceID, period string, scale float64,
) *pbc.Recommendation {
	out := &pbc.Recommendation{
		Id:         rec.ID,
		Category:   pbc.RecommendationCategory_RECOMMENDATION_CATEGORY_COST,
		ActionType: pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_OTHER,
		Resource: &pbc.ResourceRecommendationInfo{
			Id:           resourceID,
			Name:         rec.ResourceName,
			Provider:     strings.ToLower(first(rec.CloudProviders)),
			ResourceType: rec.ResourceType,
			Region:       first(rec.Regions),
		},
		Impact: &pbc.RecommendationImpact{
			EstimatedSavings: rec.CostImpact * scale,
			Currency:         p.cfg.Currency,
			ProjectionPeriod: period,
		},
		Priority:    pbc.RecommendationPriority_RECOMMENDATION_PRIORITY_UNSPECIFIED,
		Description: rec.Title,
		Source:      PluginName,
		Metadata:    recommendationMetadata(rec),
	}
	if !rec.Created.IsZero() {
		out.CreatedAt = timestamppb.New(rec.Created)
	}
	return out
}

// recommendationMetadata keeps CloudZero's own labels, which this plugin does
// not translate into action types or priorities, and the CZRN, which names the
// disk when a root-volume recommendation is shown on its instance.
func recommendationMetadata(rec czclient.Recommendation) map[string]string {
	metadata := map[string]string{}
	for key, value := range map[string]string{
		"cloudzero_type_id":   rec.TypeID,
		"cloudzero_type_name": rec.TypeName,
		"cloudzero_effort":    rec.Effort,
		"cloudzero_status":    rec.Status,
		"cloudzero_accounts":  strings.Join(rec.Accounts, ","),
		"cloudzero_resource":  rec.Resource,
	} {
		if value != "" {
			metadata[key] = value
		}
	}
	return metadata
}

// savingsScale converts CloudZero's 30-day cost impact to a projection period
// core may ask for.
func savingsScale(period string) (float64, bool) {
	switch period {
	case dailyPeriod:
		return 1.0 / daysPerMonth, true
	case monthlyPeriod:
		return 1, true
	case annualPeriod:
		return float64(daysPerYear) / daysPerMonth, true
	default:
		return 0, false
	}
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}
