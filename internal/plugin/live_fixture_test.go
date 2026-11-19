package plugin

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/rs/zerolog"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// testdata/live/costs_by_resource_and_provider.json is a CloudZero response
// captured on 2026-11-17: monthly, October and November, grouped by Resource
// and CloudProvider for one account in us-west-2. The account, the bucket name,
// and the EC2 ids are replaced. The resources below are the initial-30 dev
// stack as core sends them. NAT gateways have no ARN in Pulumi state. The
// fixture's volume ids are sanitized, so two instances are given a volume of
// the size their real root volume billed; the others carry none.
func TestBatchCostAgainstLiveFixture(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile(filepath.Join("testdata", "live", "costs_by_resource_and_provider.json"))
	require.NoError(t, err)
	var hits atomic.Int32
	plug := httpPlugin(t, readyConfig(), func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		_, _ = w.Write(body)
	}, zerolog.Nop())

	arn := func(kind, id string) *pbc.ResourceDescriptor {
		return resource("arn:aws:ec2:us-west-2:123456789012:" + kind + "/" + id)
	}
	bare := func(id string) *pbc.ResourceDescriptor {
		return &pbc.ResourceDescriptor{Provider: "aws", Region: "us-west-2", Id: id}
	}
	withRoot := func(r *pbc.ResourceDescriptor, volumeID string) *pbc.ResourceDescriptor {
		attrs, attrErr := structpb.NewStruct(map[string]any{"rootBlockDevice": map[string]any{"volumeId": volumeID}})
		require.NoError(t, attrErr)
		r.Attributes = attrs
		return r
	}
	tests := []struct {
		name     string
		resource *pbc.ResourceDescriptor
		want     float64
	}{
		{
			"windowsClientInstance",
			withRoot(arn("instance", "i-db5132d1d0af7e819"), "vol-69e15b0febeb56c6e"),
			43.83670002 + 2.990995,
		},
		{"exitNode", withRoot(arn("instance", "i-bb764cf21cd1099a7"), "vol-0009c0d553b61bc5a"), 7.592 + 0.797599},
		{"webSubnetRouter", arn("instance", "i-80f10b7b256d716c6"), 7.592},
		{"clientInstance", arn("instance", "i-519fb3d7cb42f3e80"), 7.592},
		{"clientNatGateway", bare("nat-bb15ecf33baa2692f"), 33.3606670994},
		{"webNatGateway", bare("nat-2ba4e48a2b1da1801"), 33.1346400781},
		{"clientNatEip", arn("elastic-ip", "eipalloc-64981d03f8457f32c"), 0.00017361},
		{"webNatEip", arn("elastic-ip", "eipalloc-41e53983084d5d356"), 0.00014722},
	}
	resources := make([]*pbc.ResourceDescriptor, len(tests))
	for i, tt := range tests {
		resources[i] = tt.resource
		resources[i].Region = "us-west-2"
	}
	resp, err := plug.BatchCost(context.Background(), &pbc.BatchCostRequest{
		QueryType: pbc.CostQueryType_COST_QUERY_TYPE_ACTUAL,
		Start:     timestamppb.New(day(2026, 10, 1)),
		End:       timestamppb.New(day(2026, 12, 1)),
		Resources: resources,
	})
	require.NoError(t, err)
	require.Len(t, resp.GetResults(), len(tests))
	for i, tt := range tests {
		total := 0.0
		for _, row := range resp.GetResults()[i].GetCostData().GetActualCost().GetResults() {
			total += row.GetCost()
		}
		assert.InDelta(t, tt.want, total, 1e-6, tt.name)
	}
	assert.Equal(t, int32(2), hits.Load(), "one query for account+region, one for region only")
}
