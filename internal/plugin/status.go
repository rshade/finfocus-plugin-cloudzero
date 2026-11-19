package plugin

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/rshade/finfocus-plugin-cloudzero/internal/czclient"
)

// failureCodes maps each CloudZero failure kind to the status core expects.
// The message is the kind's fixed text, never the upstream body.
var failureCodes = map[czclient.Kind]codes.Code{
	czclient.KindAuth:        codes.PermissionDenied,
	czclient.KindBadRequest:  codes.InvalidArgument,
	czclient.KindRateLimit:   codes.ResourceExhausted,
	czclient.KindBudget:      codes.ResourceExhausted,
	czclient.KindExpired:     codes.Unavailable,
	czclient.KindUnavailable: codes.Unavailable,
	czclient.KindMalformed:   codes.Unavailable,
}

func statusFrom(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return status.FromContextError(err).Err()
	}
	if api, isAPI := errors.AsType[*czclient.APIError](err); isAPI {
		if code, ok := failureCodes[api.Kind]; ok {
			return status.Error(code, api.Error())
		}
	}
	return status.Error(codes.Unavailable, "CloudZero is unavailable")
}
