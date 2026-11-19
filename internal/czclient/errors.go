package czclient

import (
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

// statedWait finds the wait in a 429 body, which reads "Please wait 24
// minute(s) before trying again".
var statedWait = regexp.MustCompile(`Please wait (\d+) minute`)

// maxStatedWait is the longest wait a 429 can honestly state: the limit is a
// rolling 24 hours. A larger number is ignored rather than reported.
const maxStatedWait = 24 * 60

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
// includes the API key, the query string, or the response body. RetryAfter is
// the wait a 429 body stated, or zero; it is the only value read from a body.
type APIError struct {
	Kind       Kind
	Status     int
	RetryAfter time.Duration
}

// Error returns the fixed message for the failure kind.
func (e *APIError) Error() string {
	switch e.Kind {
	case KindAuth:
		return "CloudZero rejected the API key"
	case KindBadRequest:
		return "CloudZero rejected the cost query"
	case KindRateLimit:
		return "CloudZero allows 60 cost requests in a rolling 24 hours; " + e.retryText()
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

// retryText rounds the wait up to whole minutes, so a wait with seconds left
// never reads shorter than it is.
func (e *APIError) retryText() string {
	minutes := int64((e.RetryAfter + time.Minute - 1) / time.Minute)
	switch {
	case minutes <= 0:
		return "retry later"
	case minutes == 1:
		return "retry in 1 minute"
	default:
		return fmt.Sprintf("retry in %d minutes", minutes)
	}
}

func newAPIError(kind Kind, code int) *APIError {
	return &APIError{Kind: kind, Status: code, RetryAfter: 0}
}

// responseError is the error for a non-200 response. A 429 keeps the wait its
// body states, and nothing else from the body.
func responseError(code int, body []byte) *APIError {
	err := newAPIError(statusKind(code), code)
	if code == http.StatusTooManyRequests {
		err.RetryAfter = statedWaitIn(body)
	}
	return err
}

// statedWaitIn is the wait a 429 body states, or zero when it states none, or
// states zero or more than one rolling window.
func statedWaitIn(body []byte) time.Duration {
	match := statedWait.FindSubmatch(body)
	if match == nil {
		return 0
	}
	minutes, err := strconv.Atoi(string(match[1]))
	if err != nil || minutes <= 0 || minutes > maxStatedWait {
		return 0
	}
	return time.Duration(minutes) * time.Minute
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
