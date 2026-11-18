package czclient

import (
	"fmt"
	"net/http"
)

// Kind classifies a CloudZero failure without forwarding the upstream status.
type Kind int

const (
	// KindAuth is a rejected key. The docs list 403. 401 is handled the same way.
	KindAuth Kind = iota
	// KindBadRequest is HTTP 400.
	KindBadRequest
	// KindRateLimit is HTTP 429. Callers must not retry in a loop.
	KindRateLimit
	// KindExpired is HTTP 410. The caller reruns the query once.
	KindExpired
	// KindUnavailable is a transport error or a 5xx that survived a short retry.
	KindUnavailable
	// KindBudget means this process has used its daily request allowance.
	KindBudget
	// KindMalformed means the response body was not the documented JSON shape.
	KindMalformed
)

// APIError is a CloudZero failure with a fixed message. The message never
// includes the API key, the query string, or the response body.
type APIError struct {
	Kind   Kind
	Status int
}

// Error returns the fixed message for the failure kind.
func (e *APIError) Error() string {
	switch e.Kind {
	case KindAuth:
		return "CloudZero rejected the API key"
	case KindBadRequest:
		return "CloudZero rejected the cost query"
	case KindRateLimit:
		return "CloudZero rate limit reached; retry tomorrow"
	case KindExpired:
		return "CloudZero cost page expired"
	case KindBudget:
		return "CloudZero cost request budget is exhausted; retry tomorrow"
	case KindMalformed:
		return "CloudZero response is not valid JSON"
	case KindUnavailable:
		return "CloudZero is unavailable"
	default:
		return fmt.Sprintf("CloudZero request failed (%d)", e.Kind)
	}
}

func newAPIError(kind Kind, code int) *APIError {
	return &APIError{Kind: kind, Status: code}
}

func statusKind(code int) Kind {
	switch code {
	case http.StatusBadRequest:
		return KindBadRequest
	case http.StatusUnauthorized, http.StatusForbidden:
		return KindAuth
	case http.StatusGone:
		return KindExpired
	case http.StatusTooManyRequests:
		return KindRateLimit
	default:
		return KindUnavailable
	}
}
