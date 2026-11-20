package plugin

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/rshade/finfocus-plugin-cloudzero/internal/config"
	"github.com/rshade/finfocus-plugin-cloudzero/internal/czclient"
)

const (
	volumeOneCZRN = "czrn:aws:ec2:us-west-2:123456789012:volume:vol-1"
	volumeTwoCZRN = "czrn:aws:ec2:us-west-2:123456789012:volume:vol-2"
	volumeOneURN  = "urn:pulumi:dev::proj::aws:ebs/volume:Volume::data-1"
	volumeTwoURN  = "urn:pulumi:dev::proj::aws:ebs/volume:Volume::data-2"
)

func recommendationOf(id, resource string, impact float64, cloudStatus string) czclient.Recommendation {
	return czclient.Recommendation{
		ID: id, Title: "Upgradable EBS Volumes", TypeID: "CIR-AWS-00010", TypeName: "Upgradable EBS Volumes",
		Effort: "low", Status: cloudStatus, Resource: resource, ResourceType: "volume", ResourceName: "",
		CostImpact: impact, Created: time.Time{}, Accounts: []string{"123456789012"},
		Regions: []string{"us-west-2"}, CloudProviders: []string{"AWS"},
	}
}

func recommendationPlugin(source *fakeCloudZero) *CloudZeroPlugin {
	cfg := readyConfig()
	cfg.ResourceDimension = ""
	return fakePlugin(cfg, source)
}

func idsOf(recs []*pbc.Recommendation) []string {
	ids := make([]string, 0, len(recs))
	for _, rec := range recs {
		ids = append(ids, rec.GetId())
	}
	return ids
}

func TestGetRecommendationsScopesToTargets(t *testing.T) {
	t.Parallel()
	source := &fakeCloudZero{recs: []czclient.Recommendation{
		recommendationOf("r-vol-1", volumeOneCZRN, 0.3, "not_started"),
		recommendationOf("r-vol-2", volumeTwoCZRN, 0.2, "in_progress"),
		recommendationOf("r-other", "czrn:aws:ec2:us-west-2:123456789012:volume:vol-9", 0.1, "not_started"),
	}}
	resp, err := recommendationPlugin(source).GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{
		TargetResources: []*pbc.ResourceDescriptor{
			{Id: volumeOneURN, Provider: "aws", Tags: map[string]string{
				"pulumi:cloudId": "vol-1", "pulumi:arn": "arn:aws:ec2:us-west-2:123456789012:volume/vol-1",
			}},
			{
				Id:       volumeTwoURN,
				Provider: "aws",
				Region:   "us-west-2",
				Tags:     map[string]string{"pulumi:cloudId": "vol-2"},
			},
			{Id: "urn:pulumi:dev::proj::random:index:RandomId::id", Provider: "aws"},
		},
	})
	require.NoError(t, err)
	got := map[string]string{}
	for _, rec := range resp.GetRecommendations() {
		got[rec.GetId()] = rec.GetResource().GetId()
	}
	assert.Equal(t, map[string]string{"r-vol-1": volumeOneURN, "r-vol-2": volumeTwoURN}, got)

	t.Run("no target resolves", func(t *testing.T) {
		t.Parallel()
		none := &fakeCloudZero{recs: source.recs}
		resp, err := recommendationPlugin(none).GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{
			TargetResources: []*pbc.ResourceDescriptor{{Id: "urn:x", Provider: "aws"}},
		})
		require.NoError(t, err)
		assert.Empty(t, resp.GetRecommendations())
		require.NotNil(t, resp.GetSummary())
		assert.Zero(t, none.recommendationFetches(), "nothing can match, so nothing is fetched")
	})
}

func TestGetRecommendationsAttachesRootVolume(t *testing.T) {
	t.Parallel()
	const instanceARN = "arn:aws:ec2:us-west-2:123456789012:instance/i-1"
	attributes := func(t *testing.T, fields map[string]any) *structpb.Struct {
		t.Helper()
		attrs, err := structpb.NewStruct(fields)
		require.NoError(t, err)
		return attrs
	}
	source := &fakeCloudZero{recs: []czclient.Recommendation{
		recommendationOf("r-root", volumeOneCZRN, 0.3, "not_started"),
		recommendationOf("r-data", volumeTwoCZRN, 0.2, "not_started"),
	}}
	tests := []struct {
		name  string
		attrs map[string]any
		want  []string
	}{
		{"object root block device", map[string]any{
			"rootBlockDevice": map[string]any{"volumeId": "vol-1"},
			"ebsBlockDevices": []any{map[string]any{"volumeId": "vol-2"}},
		}, []string{"r-root"}},
		{"list root block device", map[string]any{
			"rootBlockDevice": []any{map[string]any{"volumeId": "vol-1"}},
		}, []string{"r-root"}},
		{"no attributes", nil, []string{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			target := &pbc.ResourceDescriptor{Id: volumeOneURN, Provider: "aws", Arn: instanceARN}
			if tt.attrs != nil {
				target.Attributes = attributes(t, tt.attrs)
			}
			resp, err := recommendationPlugin(&fakeCloudZero{recs: source.recs}).GetRecommendations(
				context.Background(),
				&pbc.GetRecommendationsRequest{TargetResources: []*pbc.ResourceDescriptor{target}},
			)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.want, idsOf(resp.GetRecommendations()))
			for _, rec := range resp.GetRecommendations() {
				assert.Equal(t, volumeOneURN, rec.GetResource().GetId(), "the instance's id")
				assert.Equal(t, volumeOneCZRN, rec.GetMetadata()["cloudzero_resource"], "the disk stays named")
			}
		})
	}
}

func TestGetRecommendationsWithoutTargets(t *testing.T) {
	t.Parallel()
	source := &fakeCloudZero{recs: []czclient.Recommendation{
		recommendationOf("r-vol-1", volumeOneCZRN, 0.3, "not_started"),
		recommendationOf("r-vol-2", volumeTwoCZRN, 0.2, "in_progress"),
	}}
	resp, err := recommendationPlugin(source).GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{})
	require.NoError(t, err)
	got := map[string]string{}
	for _, rec := range resp.GetRecommendations() {
		got[rec.GetId()] = rec.GetResource().GetId()
	}
	assert.Equal(t, map[string]string{"r-vol-1": volumeOneCZRN, "r-vol-2": volumeTwoCZRN}, got)
}

func TestGetRecommendationsMapsFields(t *testing.T) {
	t.Parallel()
	body, err := os.ReadFile(filepath.Join("..", "czclient", "testdata", "live", "recommendations.json"))
	require.NoError(t, err)
	cfg := readyConfig()
	cfg.ResourceDimension = ""
	plug := httpPlugin(t, cfg, func(w http.ResponseWriter, r *http.Request) {
		page := body
		if r.URL.Query().Get("cursor") != "" {
			page = []byte(`{"recommendations":[],"pagination":{"cursor":{"next_cursor":null,"has_next":false}}}`)
		}
		_, writeErr := w.Write(page)
		assert.NoError(t, writeErr)
	}, zerolog.Nop())

	resp, err := plug.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{})
	require.NoError(t, err)
	require.Len(t, resp.GetRecommendations(), 5)
	got := resp.GetRecommendations()[0]
	assert.Equal(t, "CIR-AWS-00010-0000000000000000000000000000000000000001", got.GetId())
	assert.Equal(t, "cloudzero", got.GetSource())
	assert.Equal(t, "Upgradable EBS Volumes", got.GetDescription())
	assert.Equal(t, pbc.RecommendationCategory_RECOMMENDATION_CATEGORY_COST, got.GetCategory())
	assert.Equal(t, pbc.RecommendationActionType_RECOMMENDATION_ACTION_TYPE_OTHER, got.GetActionType())
	assert.Equal(t, pbc.RecommendationPriority_RECOMMENDATION_PRIORITY_UNSPECIFIED, got.GetPriority())
	assert.Equal(t, time.Unix(1794943043, 0).UTC(), got.GetCreatedAt().AsTime())
	assert.Equal(t, &pbc.ResourceRecommendationInfo{
		Id:           "czrn:aws:ec2:us-west-2:123456789012:volume:vol-00000000000000001",
		Name:         "vol-00000000000000001",
		Provider:     "aws",
		ResourceType: "volume",
		Region:       "us-west-2",
	}, got.GetResource())
	assert.Equal(t, map[string]string{
		"cloudzero_type_id":   "CIR-AWS-00010",
		"cloudzero_type_name": "Upgradable EBS Volumes",
		"cloudzero_effort":    "low",
		"cloudzero_status":    "not_started",
		"cloudzero_accounts":  "123456789012",
		"cloudzero_resource":  "czrn:aws:ec2:us-west-2:123456789012:volume:vol-00000000000000001",
	}, got.GetMetadata())
	assert.InDelta(t, 0.33757025732400003, got.GetImpact().GetEstimatedSavings(), 1e-12)
	assert.Equal(t, "USD", got.GetImpact().GetCurrency())
	assert.Equal(t, "monthly", got.GetImpact().GetProjectionPeriod())
	assert.Zero(t, got.GetImpact().GetCurrentCost())
	assert.Zero(t, got.GetImpact().GetProjectedCost())
}

func TestGetRecommendationsScalesSavings(t *testing.T) {
	t.Parallel()
	tests := []struct {
		period     string
		wantPeriod string
		want       float64
	}{
		{"", "monthly", 0.30},
		{"monthly", "monthly", 0.30},
		{"daily", "daily", 0.01},
		{"annual", "annual", 3.65},
	}
	for _, tt := range tests {
		t.Run("period "+tt.wantPeriod+" from "+tt.period, func(t *testing.T) {
			t.Parallel()
			source := &fakeCloudZero{
				recs: []czclient.Recommendation{recommendationOf("r", volumeOneCZRN, 0.30, "not_started")},
			}
			resp, err := recommendationPlugin(source).GetRecommendations(context.Background(),
				&pbc.GetRecommendationsRequest{ProjectionPeriod: tt.period})
			require.NoError(t, err)
			require.Len(t, resp.GetRecommendations(), 1)
			impact := resp.GetRecommendations()[0].GetImpact()
			assert.InDelta(t, tt.want, impact.GetEstimatedSavings(), 1e-9)
			assert.Equal(t, tt.wantPeriod, impact.GetProjectionPeriod())
			assert.Equal(t, tt.wantPeriod, resp.GetSummary().GetProjectionPeriod())
		})
	}

	t.Run("unknown period", func(t *testing.T) {
		t.Parallel()
		source := &fakeCloudZero{}
		_, err := recommendationPlugin(source).GetRecommendations(context.Background(),
			&pbc.GetRecommendationsRequest{ProjectionPeriod: "weekly"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
		assert.Zero(t, source.recommendationFetches())
	})
}

func TestGetRecommendationsVisibility(t *testing.T) {
	t.Parallel()
	recs := []czclient.Recommendation{
		recommendationOf("open", volumeOneCZRN, 0.3, "not_started"),
		recommendationOf("ignored", volumeTwoCZRN, 0.2, "ignored"),
		recommendationOf("addressed", volumeTwoCZRN, 0.1, "addressed"),
	}
	tests := []struct {
		name string
		req  *pbc.GetRecommendationsRequest
		want []string
	}{
		{"default", &pbc.GetRecommendationsRequest{}, []string{"open"}},
		{"include dismissed", &pbc.GetRecommendationsRequest{IncludeDismissed: true}, []string{"open", "ignored"}},
		{"excluded id", &pbc.GetRecommendationsRequest{
			IncludeDismissed: true, ExcludedRecommendationIds: []string{"open"},
		}, []string{"ignored"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			resp, err := recommendationPlugin(
				&fakeCloudZero{recs: recs},
			).GetRecommendations(context.Background(), tt.req)
			require.NoError(t, err)
			assert.ElementsMatch(t, tt.want, idsOf(resp.GetRecommendations()))
		})
	}
}

func TestGetRecommendationsPagesAndFilters(t *testing.T) {
	t.Parallel()
	recs := make([]czclient.Recommendation, 0, 5)
	for i, id := range []string{"a", "b", "c", "d", "e"} {
		recs = append(recs, recommendationOf(id, volumeOneCZRN, float64(i+1), "not_started"))
	}
	source := &fakeCloudZero{recs: recs}
	plug := recommendationPlugin(source)

	first, err := plug.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{PageSize: 3})
	require.NoError(t, err)
	assert.Len(t, first.GetRecommendations(), 3)
	assert.Equal(t, int32(3), first.GetSummary().GetTotalRecommendations(), "the summary covers the page")
	require.NotEmpty(t, first.GetNextPageToken())

	second, err := plug.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{
		PageSize: 3, PageToken: first.GetNextPageToken(),
	})
	require.NoError(t, err)
	assert.Len(t, second.GetRecommendations(), 2)
	assert.Empty(t, second.GetNextPageToken())
	assert.ElementsMatch(t, []string{"a", "b", "c", "d", "e"},
		append(idsOf(first.GetRecommendations()), idsOf(second.GetRecommendations())...))

	filtered, err := plug.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{
		Filter: &pbc.RecommendationFilter{MinEstimatedSavings: 3},
	})
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"c", "d", "e"}, idsOf(filtered.GetRecommendations()))

	_, err = plug.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{PageToken: "%%%"})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Equal(t, 1, source.recommendationFetches(), "every page reads the cached list")
}

func TestGetRecommendationsCachesOneFetch(t *testing.T) {
	t.Parallel()

	t.Run("three pages inside the window", func(t *testing.T) {
		t.Parallel()
		source := &fakeCloudZero{
			recs: []czclient.Recommendation{recommendationOf("r", volumeOneCZRN, 1, "not_started")},
		}
		plug := recommendationPlugin(source)
		for range 3 {
			_, err := plug.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{})
			require.NoError(t, err)
		}
		assert.Equal(t, 1, source.recommendationFetches())
	})

	t.Run("concurrent calls share one fetch", func(t *testing.T) {
		t.Parallel()
		gate := make(chan struct{})
		source := &fakeCloudZero{
			recs: []czclient.Recommendation{recommendationOf("r", volumeOneCZRN, 1, "not_started")}, gate: gate,
		}
		plug := recommendationPlugin(source)
		var wg sync.WaitGroup
		for range 3 {
			wg.Go(func() {
				resp, err := plug.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{})
				assert.NoError(t, err)
				assert.Len(t, resp.GetRecommendations(), 1)
			})
		}
		require.Eventually(t, func() bool {
			return plug.recsInFlight.waiting() == 3
		}, 5*time.Second, time.Millisecond)
		close(gate)
		wg.Wait()
		assert.Equal(t, 1, source.recommendationFetches())
	})
}

func TestGetRecommendationsErrors(t *testing.T) {
	t.Parallel()

	t.Run("missing key makes no call", func(t *testing.T) {
		t.Parallel()
		source := &fakeCloudZero{}
		_, err := fakePlugin(config.Config{}, source).GetRecommendations(context.Background(),
			&pbc.GetRecommendationsRequest{})
		assert.Equal(t, codes.FailedPrecondition, status.Code(err))
		assert.Zero(t, source.recommendationFetches())
	})

	tests := []struct {
		name string
		kind czclient.Kind
		want codes.Code
	}{
		{"rate limit", czclient.KindRateLimit, codes.ResourceExhausted},
		{"auth", czclient.KindAuth, codes.PermissionDenied},
		{"malformed", czclient.KindMalformed, codes.Unavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			source := &fakeCloudZero{recErr: &czclient.APIError{Kind: tt.kind}}
			plug := recommendationPlugin(source)
			for range 2 {
				_, err := plug.GetRecommendations(context.Background(), &pbc.GetRecommendationsRequest{})
				assert.Equal(t, tt.want, status.Code(err))
			}
			assert.Equal(t, 2, source.recommendationFetches(), "a failure is not cached")
		})
	}
}

func TestRecommendationWritesUnimplemented(t *testing.T) {
	t.Parallel()
	source := &fakeCloudZero{}
	logger := zerolog.Nop()
	server := pluginsdk.NewServerWithOptions(recommendationPlugin(source), nil, &logger, Info("vtest"))

	_, err := server.DismissRecommendation(context.Background(), &pbc.DismissRecommendationRequest{
		RecommendationId: "r",
	})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
	_, err = server.GetRecommendationHistory(context.Background(), &pbc.GetRecommendationHistoryRequest{
		RecommendationId: "r",
	})
	assert.Equal(t, codes.Unimplemented, status.Code(err))
	assert.Zero(t, source.recommendationFetches())
}
