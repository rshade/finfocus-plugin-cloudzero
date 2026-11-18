package conformance_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/rshade/finfocus-spec/sdk/go/pluginsdk"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	plugintesting "github.com/rshade/finfocus-spec/sdk/go/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rshade/finfocus-plugin-cloudzero/internal/config"
	"github.com/rshade/finfocus-plugin-cloudzero/internal/czclient"
	"github.com/rshade/finfocus-plugin-cloudzero/internal/plugin"
)

func TestSDKConformance(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"costs":[],"pagination":{"cursor":{"has_next":false}}}`))
	}))
	t.Cleanup(srv.Close)

	cfg := config.Config{
		APIKey: "conformance-key", BaseURL: srv.URL, CostType: config.DefaultCostType,
		Currency: config.DefaultCurrency, ResourceDimension: "Resource",
		DailyBudget: 50, CacheTTL: time.Hour, RecentTTL: time.Minute,
	}
	logger := zerolog.Nop()
	impl := plugin.New("0.1.0", cfg, czclient.New(cfg, "0.1.0", logger), logger)
	info := plugin.Info("0.1.0")
	server := pluginsdk.NewServerWithOptions(impl, nil, &logger, info)

	sample := &pbc.ResourceDescriptor{
		Provider:     "aws",
		ResourceType: "aws:ec2/instance:Instance",
		Region:       "us-east-1",
		Arn:          "arn:aws:ec2:us-east-1:123456789012:instance/i-conformance",
	}
	result, err := plugintesting.RunConformance(
		server, plugintesting.ConformanceLevelBasic, plugintesting.WithSampleResource(sample),
	)
	require.NoError(t, err)
	for _, category := range result.Categories {
		for _, item := range category.Results {
			assert.Truef(t, item.Success, "%s/%s %s err=%v", item.Category, item.Method, item.Details, item.Error)
		}
	}
	require.Zero(t, result.Summary.Failed)
}
