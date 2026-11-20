package czclient

import (
	"context"
	"encoding/json"
	"net/url"
	"time"
)

// recommendationFloor is the lowest cost_impact_last_30_days[gte] CloudZero
// accepts. Its default of 1.00 hides sub-dollar recommendations.
const recommendationFloor = "0.01"

// Recommendation is one element of GET /optimize/recommendations. Resource is
// a CZRN. CostImpact is CloudZero's cost_impact_last_30_days, or zero when it
// is not a number. Created is zero when CloudZero sent none.
type Recommendation struct {
	ID             string
	Title          string
	TypeID         string
	TypeName       string
	Effort         string
	Status         string
	Resource       string
	ResourceType   string
	ResourceName   string
	CostImpact     float64
	Created        time.Time
	Accounts       []string
	Regions        []string
	CloudProviders []string
}

// GetRecommendations calls GET /optimize/recommendations for every status that
// can be shown (all but addressed), above a one-cent floor, and follows
// cursors. The first page spends budget and respects the rate-limit cooldown
// like a costs first page. Cursor pages do not spend budget.
func (c *Client) GetRecommendations(ctx context.Context) ([]Recommendation, error) {
	return fetchAll(ctx, c, "/optimize/recommendations", recommendationQuery, parseRecommendations)
}

func recommendationQuery(cursor string) url.Values {
	v := url.Values{}
	v.Set("cost_impact_last_30_days[gte]", recommendationFloor)
	v["status"] = []string{"not_started", "in_progress", "ignored"}
	if cursor != "" {
		v.Set("cursor", cursor)
	}
	return v
}

type rawRecommendation struct {
	ID            string   `json:"recommendation_id"`
	Title         string   `json:"title"`
	TypeID        string   `json:"recommendation_type_id"`
	TypeName      string   `json:"recommendation_type_name"`
	Effort        string   `json:"effort"`
	Status        string   `json:"status"`
	Resource      string   `json:"resource"`
	ResourceType  string   `json:"resource_type"`
	ResourceName  string   `json:"resource_name"`
	CostImpact    any      `json:"cost_impact_last_30_days"`
	Created       any      `json:"created"`
	Account       []string `json:"account"`
	Region        []string `json:"region"`
	CloudProvider []string `json:"cloud_provider"`
}

func parseRecommendations(body []byte) ([]Recommendation, string, bool, error) {
	var payload struct {
		Recommendations []rawRecommendation `json:"recommendations"`
		Pagination      struct {
			Cursor struct {
				NextCursor *string `json:"next_cursor"`
				HasNext    bool    `json:"has_next"`
			} `json:"cursor"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, "", false, newAPIError(KindMalformed, 0)
	}
	recs := make([]Recommendation, 0, len(payload.Recommendations))
	for _, raw := range payload.Recommendations {
		if raw.ID == "" || raw.Resource == "" {
			continue
		}
		recs = append(recs, raw.recommendation())
	}
	next := ""
	if payload.Pagination.Cursor.NextCursor != nil {
		next = *payload.Pagination.Cursor.NextCursor
	}
	return recs, next, payload.Pagination.Cursor.HasNext, nil
}

func (raw rawRecommendation) recommendation() Recommendation {
	impact, err := floatValue(raw.CostImpact)
	if err != nil {
		impact = 0
	}
	var created time.Time
	if seconds, parseErr := floatValue(raw.Created); parseErr == nil && seconds > 0 {
		created = time.Unix(int64(seconds), 0).UTC()
	}
	return Recommendation{
		ID:             raw.ID,
		Title:          raw.Title,
		TypeID:         raw.TypeID,
		TypeName:       raw.TypeName,
		Effort:         raw.Effort,
		Status:         raw.Status,
		Resource:       raw.Resource,
		ResourceType:   raw.ResourceType,
		ResourceName:   raw.ResourceName,
		CostImpact:     impact,
		Created:        created,
		Accounts:       raw.Account,
		Regions:        raw.Region,
		CloudProviders: raw.CloudProvider,
	}
}
