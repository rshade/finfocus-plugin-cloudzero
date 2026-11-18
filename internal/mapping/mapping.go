// Package mapping turns a FinFocus resource into CloudZero dimension filters.
package mapping

import (
	"errors"
	"maps"
	"strings"

	"github.com/rs/zerolog"
	pbc "github.com/rshade/finfocus-spec/sdk/go/proto/finfocus/v1"
)

// Filter is the CloudZero slice for one resource.
type Filter struct {
	// Shared are dimension IDs other than the resource dimension.
	Shared map[string]string
	// ResourceID is the value of the configured resource dimension.
	ResourceID string
}

// Mapper builds CloudZero filters from a resource descriptor.
type Mapper struct {
	logger            zerolog.Logger
	tagDimensions     map[string]string
	resourceDimension string
}

// NewMapper returns a mapper. tagDimensions maps a Pulumi tag key to a
// CloudZero dimension ID. resourceDimension is the dimension that identifies
// one resource. Both come from configuration. Nothing is inferred from tag names.
func NewMapper(logger zerolog.Logger, tagDimensions map[string]string, resourceDimension string) *Mapper {
	copied := make(map[string]string, len(tagDimensions))
	maps.Copy(copied, tagDimensions)
	return &Mapper{logger: logger, tagDimensions: copied, resourceDimension: resourceDimension}
}

// Filter builds filters for a descriptor. Identifiers are tried in order: the
// descriptor's pulumi:cloudId tag, ARN, and ID, then the legacy tags, arn,
// and resourceID fields. The first one that yields a CZRN identifies the
// resource.
func (m *Mapper) Filter(
	resource *pbc.ResourceDescriptor,
	tags map[string]string,
	arn, resourceID string,
) (Filter, error) {
	if m.resourceDimension == "" {
		return Filter{}, errors.New("resource dimension is not configured")
	}
	ids := identifiersOf(resource, tags, arn, resourceID)
	region := ids.region(resource)
	id, ok := ids.czrn(region)
	if !ok {
		return Filter{}, errors.New("cannot build a czrn for this resource")
	}
	shared := map[string]string{}
	if provider, known := cloudProvider(resource.GetProvider()); known {
		shared["CloudProvider"] = provider
	}
	if account := ids.account(); account != "" {
		shared["Account"] = account
	} else if ids.firstARN() != "" {
		m.logger.Warn().Msg("invalid arn format")
	}
	if region != "" {
		shared["Region"] = region
	}
	m.addTagFilters(shared, resource, tags)
	return Filter{Shared: shared, ResourceID: id}, nil
}

func (m *Mapper) addTagFilters(shared map[string]string, resource *pbc.ResourceDescriptor, tags map[string]string) {
	for tagKey, dimensionID := range m.tagDimensions {
		value := resource.GetTags()[tagKey]
		if value == "" {
			value = tags[tagKey]
		}
		if value != "" {
			shared[dimensionID] = value
		}
	}
}

// identifiers lists the candidate resource identifiers in precedence order.
type identifiers []string

func identifiersOf(resource *pbc.ResourceDescriptor, tags map[string]string, arn, resourceID string) identifiers {
	return identifiers{
		resource.GetTags()[cloudIDTag], resource.GetArn(), resource.GetId(),
		tags[cloudIDTag], arn, resourceID,
	}
}

func (ids identifiers) czrn(fallbackRegion string) (string, bool) {
	for _, id := range ids {
		if czrn, ok := resourceCZRN(id, fallbackRegion); ok {
			return czrn, true
		}
	}
	return "", false
}

func (ids identifiers) firstARN() string {
	for _, id := range ids {
		if strings.HasPrefix(id, arnPrefix) {
			return id
		}
	}
	return ""
}

func (ids identifiers) account() string {
	for _, id := range ids {
		if account := accountFromIdentifier(id); account != "" {
			return account
		}
	}
	return ""
}

func (ids identifiers) region(resource *pbc.ResourceDescriptor) string {
	if resource.GetRegion() != "" {
		return resource.GetRegion()
	}
	if fields, ok := arnFields(ids.firstARN()); ok {
		return fields.region
	}
	return ""
}

// cloudProvider maps a FinFocus provider name to the CloudZero CloudProvider
// value. AWS is the documented example. Azure and GCP spellings are assumptions
// until a live capture confirms them.
func cloudProvider(name string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "aws", "aws-native":
		return "AWS", true
	case "azure", "azure-native", "azurerm":
		return "Azure", true
	case "gcp", "google", "google-native":
		return "GCP", true
	default:
		return "", false
	}
}

// SupportedProvider reports whether the provider is one this plugin can bill.
func SupportedProvider(name string) bool {
	_, ok := cloudProvider(name)
	return ok
}

func accountFromIdentifier(id string) string {
	if strings.HasPrefix(id, arnPrefix) {
		fields, _ := arnFields(id)
		return fields.account
	}
	if account := pathValue(id, "subscriptions"); account != "" {
		return account
	}
	return pathValue(id, "projects")
}

func pathValue(id, key string) string {
	parts := strings.Split(id, "/")
	for i, part := range parts {
		if part == key && i+1 < len(parts) && parts[i+1] != "" {
			return parts[i+1]
		}
	}
	return ""
}
