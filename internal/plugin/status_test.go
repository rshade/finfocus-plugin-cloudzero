package plugin

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/rshade/finfocus-plugin-cloudzero/internal/czclient"
)

func TestStatusFrom(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"auth", &czclient.APIError{Kind: czclient.KindAuth}, codes.PermissionDenied},
		{"bad request", &czclient.APIError{Kind: czclient.KindBadRequest}, codes.InvalidArgument},
		{"rate limit", &czclient.APIError{Kind: czclient.KindRateLimit}, codes.ResourceExhausted},
		{"budget", &czclient.APIError{Kind: czclient.KindBudget}, codes.ResourceExhausted},
		{"expired", &czclient.APIError{Kind: czclient.KindExpired}, codes.Unavailable},
		{"malformed", &czclient.APIError{Kind: czclient.KindMalformed}, codes.Unavailable},
		{"unavailable", &czclient.APIError{Kind: czclient.KindUnavailable}, codes.Unavailable},
		{"unknown kind", &czclient.APIError{Kind: czclient.Kind(99)}, codes.Unavailable},
		{"not an api error", assert.AnError, codes.Unavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := statusFrom(tt.err)
			assert.Equal(t, tt.want, status.Code(got))
			assert.NotContains(t, status.Convert(got).Message(), assert.AnError.Error())
		})
	}
}
