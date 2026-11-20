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
	// RootVolumeSuffix is ":volume:<vol-id>" for an AWS instance whose
	// attributes name its root volume, which CloudZero bills as its own
	// resource. Empty when there is none.
	RootVolumeSuffix string
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

// MatchesRootVolume reports whether a CloudZero row is this resource's root
// volume. The rows come from the resource's own account and region, so the
// volume id suffix is enough.
func (f Filter) MatchesRootVolume(resource, cloudProvider string) bool {
	if f.RootVolumeSuffix == "" {
		return false
	}
	if f.Provider != "" && !strings.EqualFold(cloudProvider, f.Provider) {
		return false
	}
	return strings.HasSuffix(resource, f.RootVolumeSuffix)
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

// Identity resolves a descriptor's CZRN, or its AWS suffix, its provider, and
// an instance's root volume, in the same order and with the same rules as
// Filter. It needs no resource
// dimension and builds no shared filters, so it suits CloudZero records that
// name their resource directly, such as recommendations.
func (m *Mapper) Identity(resource *pbc.ResourceDescriptor) (Filter, error) {
	filter, _, _, err := m.identity(resource, nil, "", "")
	return filter, err
}

// Filter builds the cost filter for a descriptor. Identifiers are tried in
// order: the descriptor's pulumi:cloudId tag, ARN, pulumi:arn and arn tags,
// and ID, then the legacy tags, arn, and resourceID fields. The first one that
// yields a CZRN identifies the resource. When none does, an AWS resource with
// a known id prefix and a region is matched by CZRN suffix instead.
func (m *Mapper) Filter(
	resource *pbc.ResourceDescriptor,
	tags map[string]string,
	arn, resourceID string,
) (Filter, error) {
	if m.resourceDimension == "" {
		return Filter{}, errors.New("resource dimension is not configured")
	}
	filter, ids, region, err := m.identity(resource, tags, arn, resourceID)
	if err != nil {
		return Filter{}, err
	}
	shared := map[string]string{}
	if account := ids.account(); account != "" {
		shared["Account"] = account
	} else if ids.firstARN() != "" {
		m.logger.Warn().Msg("invalid arn format")
	}
	if region != "" {
		shared["Region"] = region
	}
	m.addTagFilters(shared, resource, tags)
	filter.Shared = shared
	return filter, nil
}

// identity resolves the resource id or suffix, the provider, and an AWS
// resource's root volume, and returns the identifiers and region it used so
// Filter can build shared filters.
func (m *Mapper) identity(
	resource *pbc.ResourceDescriptor,
	tags map[string]string,
	arn, resourceID string,
) (Filter, identifiers, string, error) {
	ids := identifiersOf(resource, tags, arn, resourceID)
	region := ids.region(resource)
	provider, _ := providerKey(resource.GetProvider())
	filter := Filter{Shared: nil, ResourceID: "", ResourceSuffix: "", Provider: provider, RootVolumeSuffix: ""}
	if id, ok := ids.czrn(region); ok {
		filter.ResourceID = id
	} else if suffix, found := ids.awsSuffix(); found && provider == awsProvider && region != "" {
		filter.ResourceSuffix = suffix
	} else {
		return Filter{}, nil, "", errors.New("cannot build a czrn for this resource")
	}
	if provider == awsProvider {
		filter.RootVolumeSuffix = rootVolumeSuffix(resource)
	}
	return filter, ids, region, nil
}

// rootVolumeSuffix reads attributes.rootBlockDevice.volumeId, an object or a
// list whose first entry holds it, as Pulumi state records an instance's root
// volume. Data volumes in ebsBlockDevices are not read.
func rootVolumeSuffix(resource *pbc.ResourceDescriptor) string {
	device := resource.GetAttributes().GetFields()["rootBlockDevice"]
	if list := device.GetListValue(); list != nil {
		if len(list.GetValues()) == 0 {
			return ""
		}
		device = list.GetValues()[0]
	}
	id := device.GetStructValue().GetFields()["volumeId"].GetStringValue()
	if !strings.HasPrefix(id, "vol-") || strings.ContainsAny(id, ":/") {
		return ""
	}
	return ":" + awsIDTypes["vol"] + ":" + id
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
	descriptorTags := resource.GetTags()
	return identifiers{
		descriptorTags[cloudIDTag], resource.GetArn(), descriptorTags[pulumiARNTag], descriptorTags[arnTag],
		resource.GetId(), tags[cloudIDTag], arn, resourceID,
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
