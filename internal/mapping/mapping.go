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
	// ResourceID is the exact value of the configured resource dimension. It is
	// empty when only ResourceSuffix is known.
	ResourceID string
	// ResourceSuffix is ":<type>:<id>" for an AWS resource that has no ARN,
	// such as a NAT gateway. Its account is unknown, so rows match on the end
	// of their CZRN, and Shared has no Account.
	ResourceSuffix string
	// Provider is the resource's provider, normalized to aws, azure, or gcp.
	// CloudZero's CloudProvider filter is case-sensitive and its spellings are
	// not documented, so the query groups by CloudProvider instead of
	// filtering on it, and rows match on that value ignoring case. Empty means
	// the request carried no provider.
	Provider string
}

// Matches reports whether a CloudZero row belongs to this resource, given the
// row's resource dimension value and its CloudProvider value.
func (f Filter) Matches(resource, cloudProvider string) bool {
	if f.Provider != "" && !strings.EqualFold(cloudProvider, f.Provider) {
		return false
	}
	if f.ResourceID != "" {
		return resource == f.ResourceID
	}
	return f.ResourceSuffix != "" && strings.HasSuffix(resource, f.ResourceSuffix)
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
// resource. When none does, an AWS resource with a known id prefix and a
// region is matched by CZRN suffix instead.
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
	provider, _ := providerKey(resource.GetProvider())
	filter := Filter{Shared: map[string]string{}, ResourceID: "", ResourceSuffix: "", Provider: provider}
	if id, ok := ids.czrn(region); ok {
		filter.ResourceID = id
	} else if suffix, found := ids.awsSuffix(); found && provider == awsProvider && region != "" {
		filter.ResourceSuffix = suffix
	} else {
		return Filter{}, errors.New("cannot build a czrn for this resource")
	}
	shared := filter.Shared
	if account := ids.account(); account != "" {
		shared["Account"] = account
	} else if ids.firstARN() != "" {
		m.logger.Warn().Msg("invalid arn format")
	}
	if region != "" {
		shared["Region"] = region
	}
	m.addTagFilters(shared, resource, tags)
	return filter, nil
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

// awsSuffix returns ":<type>:<id>" for the first identifier that is a bare AWS
// id with a known prefix, such as nat-0abc.
func (ids identifiers) awsSuffix() (string, bool) {
	for _, id := range ids {
		prefix, rest, ok := strings.Cut(id, "-")
		if !ok || rest == "" || strings.ContainsAny(rest, ":/") {
			continue
		}
		if czrnType, known := awsIDTypes[prefix]; known {
			return ":" + czrnType + ":" + id, true
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

const (
	azureProvider = "azure"
	gcpProvider   = "gcp"
)

// providerAliases folds Pulumi and Terraform provider names onto one name per
// cloud. These are FinFocus provider names, not CloudZero values.
var providerAliases = map[string]string{
	awsProvider:   awsProvider,
	azureProvider: azureProvider,
	"azurerm":     azureProvider,
	gcpProvider:   gcpProvider,
	"google":      gcpProvider,
}

// providerKey normalizes a FinFocus provider name such as aws-native or
// google to aws, azure, or gcp.
func providerKey(name string) (string, bool) {
	base := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), "-native")
	key, ok := providerAliases[base]
	return key, ok
}

// SupportedProvider reports whether the provider is one this plugin can bill.
func SupportedProvider(name string) bool {
	_, ok := providerKey(name)
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
