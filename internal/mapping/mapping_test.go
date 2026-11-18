package mapping

import (
	"testing"

	"github.com/rs/zerolog"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFilterCloudIdentifiers(t *testing.T) {
	t.Parallel()
	mapper := NewMapper(zerolog.Nop(), map[string]string{"Team": "User:Defined:Team"}, "Resource")
	awsARN := "arn:aws:ec2:us-east-1:123456789012:instance/i-abc"

	tests := []struct {
		name     string
		resource *pbc.ResourceDescriptor
		want     Filter
	}{
		{
			name: "aws arn",
			resource: &pbc.ResourceDescriptor{
				Provider: "aws",
				Region:   "us-east-1",
				Arn:      awsARN,
				Tags:     map[string]string{"Team": "platform", "cost_center": "nope"},
			},
			want: Filter{
				ResourceID: "czrn:aws:ec2:us-east-1:123456789012:instance:i-abc",
				Shared: map[string]string{
					"CloudProvider": "AWS", "Account": "123456789012", "Region": "us-east-1",
					"User:Defined:Team": "platform",
				},
			},
		},
		{
			name: "nat gateway",
			resource: &pbc.ResourceDescriptor{
				Provider: "aws",
				Arn:      "arn:aws:ec2:us-west-2:123456789012:natgateway/nat-abc",
			},
			want: Filter{
				ResourceID: "czrn:aws:ec2:us-west-2:123456789012:nat-gateway:nat-abc",
				Shared: map[string]string{
					"CloudProvider": "AWS", "Account": "123456789012", "Region": "us-west-2",
				},
			},
		},
		{
			name: "s3 bucket uses descriptor region",
			resource: &pbc.ResourceDescriptor{
				Provider: "aws",
				Region:   "us-east-1",
				Arn:      "arn:aws:s3:us-east-1:123456789012:photos",
			},
			want: Filter{
				ResourceID: "czrn:aws:s3:us-east-1:123456789012:bucket:photos",
				Shared: map[string]string{
					"CloudProvider": "AWS", "Account": "123456789012", "Region": "us-east-1",
				},
			},
		},
		{
			name: "existing czrn wins over arn",
			resource: &pbc.ResourceDescriptor{
				Provider: "aws",
				Arn:      awsARN,
				Tags: map[string]string{
					"pulumi:cloudId": "czrn:aws:ec2:us-east-1:123456789012:instance:i-from-tag",
				},
			},
			want: Filter{
				ResourceID: "czrn:aws:ec2:us-east-1:123456789012:instance:i-from-tag",
				Shared: map[string]string{
					"CloudProvider": "AWS", "Account": "123456789012", "Region": "us-east-1",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := mapper.Filter(tt.resource, nil, "", "")
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFilterLegacyFieldsWhenResourceMissing(t *testing.T) {
	t.Parallel()
	mapper := NewMapper(zerolog.Nop(), nil, "Resource")
	_, err := mapper.Filter(nil, map[string]string{"pulumi:cloudId": "i-legacy"}, "", "ignored")
	require.Error(t, err)

	got, err := mapper.Filter(nil, nil, "arn:aws:ec2:eu-west-1:999:instance/i-1", "")
	require.NoError(t, err)
	assert.Equal(t, "czrn:aws:ec2:eu-west-1:999:instance:i-1", got.ResourceID)
	assert.Equal(t, "999", got.Shared["Account"])
	assert.Equal(t, "eu-west-1", got.Shared["Region"])
}

func TestFilterIdentityPrecedence(t *testing.T) {
	t.Parallel()
	mapper := NewMapper(zerolog.Nop(), nil, "Resource")
	const (
		ec2ARN     = "arn:aws:ec2:us-east-1:123456789012:instance/i-arn"
		legacyCZRN = "czrn:aws:ec2:us-east-1:123456789012:instance:i-legacy"
		azureID    = "/subscriptions/sub-1/resourceGroups/rg/providers/Microsoft.Compute/virtualMachines/vm"
		azureCZRN  = "czrn:azure:compute:eastus:sub-1:virtualmachines:vm"
	)
	tests := []struct {
		name       string
		resource   *pbc.ResourceDescriptor
		tags       map[string]string
		arn        string
		resourceID string
		wantID     string
		wantShared map[string]string
	}{
		{
			name:     "arn in pulumi cloudId tag",
			resource: &pbc.ResourceDescriptor{Provider: "aws", Tags: map[string]string{"pulumi:cloudId": ec2ARN}},
			wantID:   "czrn:aws:ec2:us-east-1:123456789012:instance:i-arn",
			wantShared: map[string]string{
				"CloudProvider": "AWS", "Account": "123456789012", "Region": "us-east-1",
			},
		},
		{
			name:       "descriptor arn beats a legacy czrn",
			resource:   &pbc.ResourceDescriptor{Provider: "aws", Arn: ec2ARN},
			resourceID: legacyCZRN,
			wantID:     "czrn:aws:ec2:us-east-1:123456789012:instance:i-arn",
			wantShared: map[string]string{
				"CloudProvider": "AWS", "Account": "123456789012", "Region": "us-east-1",
			},
		},
		{
			name:       "legacy cloudId tag without a descriptor",
			tags:       map[string]string{"pulumi:cloudId": legacyCZRN},
			wantID:     legacyCZRN,
			wantShared: map[string]string{},
		},
		{
			name: "azure czrn with subscription from id",
			resource: &pbc.ResourceDescriptor{
				Provider: "azure-native", Region: "eastus", Id: azureID,
				Tags: map[string]string{"pulumi:cloudId": azureCZRN},
			},
			wantID:     azureCZRN,
			wantShared: map[string]string{"CloudProvider": "Azure", "Account": "sub-1", "Region": "eastus"},
		},
		{
			name: "gcp czrn with project from id",
			resource: &pbc.ResourceDescriptor{
				Provider: "gcp", Id: "projects/proj-1/zones/us-central1-a/instances/vm",
				Tags: map[string]string{"pulumi:cloudId": "czrn:gcp:compute:us-central1:proj-1:instance:vm"},
			},
			wantID:     "czrn:gcp:compute:us-central1:proj-1:instance:vm",
			wantShared: map[string]string{"CloudProvider": "GCP", "Account": "proj-1"},
		},
		{
			name: "short arn is ignored for account",
			resource: &pbc.ResourceDescriptor{
				Provider: "aws", Arn: "arn:aws:ec2",
				Tags: map[string]string{"pulumi:cloudId": legacyCZRN},
			},
			wantID:     legacyCZRN,
			wantShared: map[string]string{"CloudProvider": "AWS"},
		},
		{
			name:       "global iam role is cross-region",
			arn:        "arn:aws:iam::123456789012:role/admin",
			wantID:     "czrn:aws:iam:cross-region:123456789012:role:admin",
			wantShared: map[string]string{"Account": "123456789012"},
		},
		{
			name:       "china partition is aws",
			arn:        "arn:aws-cn:ec2:cn-north-1:123456789012:instance/i-cn",
			wantID:     "czrn:aws:ec2:cn-north-1:123456789012:instance:i-cn",
			wantShared: map[string]string{"Account": "123456789012", "Region": "cn-north-1"},
		},
		{
			name:       "other partition keeps its name",
			arn:        "arn:other:sqs:eu-west-1:123456789012:queue:jobs",
			wantID:     "czrn:other:sqs:eu-west-1:123456789012:queue:jobs",
			wantShared: map[string]string{"Account": "123456789012", "Region": "eu-west-1"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := mapper.Filter(tt.resource, tt.tags, tt.arn, tt.resourceID)
			require.NoError(t, err)
			assert.Equal(t, tt.wantID, got.ResourceID)
			assert.Equal(t, tt.wantShared, got.Shared)
		})
	}
}

func TestFilterRejectsMissingIdentity(t *testing.T) {
	t.Parallel()
	mapper := NewMapper(zerolog.Nop(), nil, "Resource")
	_, err := mapper.Filter(&pbc.ResourceDescriptor{Provider: "aws"}, nil, "", "")
	require.Error(t, err)
	_, err = mapper.Filter(&pbc.ResourceDescriptor{
		Provider: "aws", Arn: "arn:aws:s3:::bucket-without-account",
	}, nil, "", "")
	require.Error(t, err)
	_, err = mapper.Filter(&pbc.ResourceDescriptor{
		Provider: "aws", Arn: "arn:aws:ec2:us-east-1:123456789012:instance",
	}, nil, "", "")
	require.Error(t, err)

	bare := NewMapper(zerolog.Nop(), nil, "")
	_, err = bare.Filter(&pbc.ResourceDescriptor{Arn: "arn:aws:ec2:us-east-1:1:instance/i"}, nil, "", "")
	require.Error(t, err)
}

func TestSupportedProvider(t *testing.T) {
	t.Parallel()
	assert.True(t, SupportedProvider("google-native"))
	assert.False(t, SupportedProvider("kubernetes"))
	assert.False(t, SupportedProvider(""))
}
