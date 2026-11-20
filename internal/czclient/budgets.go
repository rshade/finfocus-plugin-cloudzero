package czclient

import (
	"context"
	"encoding/json"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

const maxAlertPercent = 100

// Budget is one element of GET /budgets read with expand=current. An amount
// CloudZero sent as neither a number nor a numeric string is nil, or absent
// from Monthly.
type Budget struct {
	ID          string
	Name        string
	ViewID      string
	Granularity string
	// Monthly maps the first instant of a UTC month to its planned amount.
	Monthly map[time.Time]float64
	// Alerts are the enabled alert percentages, ascending.
	Alerts  []float64
	Planned *float64
	Actual  *float64
}

// GetBudgets calls GET /budgets with expand=current and follows cursors. The
// first page spends budget and respects the rate-limit cooldown like a costs
// first page. Cursor pages do not spend budget.
func (c *Client) GetBudgets(ctx context.Context) ([]Budget, error) {
	return fetchAll(ctx, c, "/budgets", budgetQuery, parseBudgets)
}

func budgetQuery(cursor string) url.Values {
	v := url.Values{}
	v.Set("expand", "current")
	if cursor != "" {
		v.Set("cursor", cursor)
	}
	return v
}

type rawBudget struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Granularity string `json:"granularity"`
	View        *struct {
		ID string `json:"id"`
	} `json:"view"`
	PlannedLimits struct {
		Monthly map[string]struct {
			Amount any `json:"amount"`
		} `json:"monthly"`
	} `json:"planned_limits"`
	Alerts  map[string]any `json:"alerts"`
	Current *struct {
		Planned any `json:"planned"`
		Actual  any `json:"actual"`
	} `json:"current"`
}

func parseBudgets(body []byte) ([]Budget, string, bool, error) {
	var payload struct {
		Budgets    []rawBudget `json:"budgets"`
		Pagination struct {
			Cursor struct {
				NextCursor *string `json:"next_cursor"`
				HasNext    bool    `json:"has_next"`
			} `json:"cursor"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, "", false, newAPIError(KindMalformed, 0)
	}
	budgets := make([]Budget, 0, len(payload.Budgets))
	for _, raw := range payload.Budgets {
		budgets = append(budgets, raw.budget())
	}
	next := ""
	if payload.Pagination.Cursor.NextCursor != nil {
		next = *payload.Pagination.Cursor.NextCursor
	}
	return budgets, next, payload.Pagination.Cursor.HasNext, nil
}

func (raw rawBudget) budget() Budget {
	b := Budget{
		ID:          raw.ID,
		Name:        raw.Name,
		ViewID:      "",
		Granularity: raw.Granularity,
		Monthly:     make(map[time.Time]float64, len(raw.PlannedLimits.Monthly)),
		Alerts:      enabledAlerts(raw.Alerts),
		Planned:     nil,
		Actual:      nil,
	}
	if raw.View != nil {
		b.ViewID = raw.View.ID
	}
	for key, limit := range raw.PlannedLimits.Monthly {
		month, ok := monthOf(key)
		if !ok {
			continue
		}
		if amount, err := floatValue(limit.Amount); err == nil {
			b.Monthly[month] = amount
		}
	}
	if raw.Current != nil {
		b.Planned = optionalFloat(raw.Current.Planned)
		b.Actual = optionalFloat(raw.Current.Actual)
	}
	return b
}

// monthOf reads a planned-limit key, YYYY-MM-DD or YYYY-MM, as the first
// instant of its UTC month.
func monthOf(key string) (time.Time, bool) {
	for _, layout := range []string{time.DateOnly, "2006-01"} {
		if day, err := time.Parse(layout, strings.TrimSpace(key)); err == nil {
			return time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC), true
		}
	}
	return time.Time{}, false
}

// enabledAlerts keeps alerts switched on ("Y" or true) whose key is a
// percentage from 1 to 100, in ascending order.
func enabledAlerts(alerts map[string]any) []float64 {
	out := make([]float64, 0, len(alerts))
	for key, value := range alerts {
		if !alertOn(value) {
			continue
		}
		percent, err := strconv.ParseFloat(strings.TrimSpace(key), 64)
		if err != nil || percent < 1 || percent > maxAlertPercent {
			continue
		}
		out = append(out, percent)
	}
	slices.Sort(out)
	return out
}

func alertOn(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(strings.TrimSpace(v), "Y")
	default:
		return false
	}
}

func optionalFloat(v any) *float64 {
	n, err := floatValue(v)
	if err != nil {
		return nil
	}
	return &n
}
