package mapping

import (
	"strings"
)

const (
	czrnPrefix  = "czrn:"
	arnPrefix   = "arn:"
	cloudIDTag  = "pulumi:cloudId"
	arnParts    = 6
	crossRegion = "cross-region"
	awsProvider = "aws"
	s3Service   = "s3"
	bucketType  = "bucket"

	awsCloudProvider = "AWS"
)

// awsIDTypes maps the prefix of a bare AWS id to its CZRN resource type. The
// pairs were observed in CloudZero CZRNs on 2026-11-17.
var awsIDTypes = map[string]string{
	"i":        "instance",
	"vol":      "volume",
	"eni":      "network-interface",
	"nat":      "nat-gateway",
	"eipalloc": "elastic-ip",
}

// arn holds the colon-separated fields of an ARN. resource keeps any colons
// that follow the fifth separator.
type arn struct {
	partition, service, region, account, resource string
}

func arnFields(id string) (arn, bool) {
	var none arn
	if !strings.HasPrefix(id, arnPrefix) {
		return none, false
	}
	parts := strings.SplitN(id, ":", arnParts)
	if len(parts) < arnParts {
		return none, false
	}
	return arn{partition: parts[1], service: parts[2], region: parts[3], account: parts[4], resource: parts[5]}, true
}

// resourceCZRN returns the CloudZero resource name used as the Resource
// dimension value. An existing czrn: id is kept. An AWS ARN is converted to
// czrn:<provider>:<service>:<region>:<account>:<type>:<id>. CloudZero does not
// document the CZRN format, so that shape is unverified until a live capture.
func resourceCZRN(resourceID, fallbackRegion string) (string, bool) {
	if strings.HasPrefix(resourceID, czrnPrefix) {
		return resourceID, true
	}
	return czrnFromARN(resourceID, fallbackRegion)
}

func czrnFromARN(id, fallbackRegion string) (string, bool) {
	fields, ok := arnFields(id)
	if !ok || fields.service == "" || fields.account == "" || fields.resource == "" {
		return "", false
	}
	region := fields.region
	if region == "" {
		region = fallbackRegion
	}
	if region == "" {
		region = crossRegion
	}
	resType, localID := arnResource(fields.service, fields.resource)
	if resType == "" || localID == "" {
		return "", false
	}
	provider := awsProvider
	if fields.partition != "aws" && fields.partition != "aws-us-gov" && fields.partition != "aws-cn" {
		provider = fields.partition
	}
	return strings.Join(
		[]string{"czrn", provider, fields.service, region, fields.account, czrnType(resType), localID}, ":",
	), true
}

func arnResource(service, resource string) (string, string) {
	if service == s3Service && !strings.Contains(resource, "/") && !strings.Contains(resource, ":") {
		return bucketType, resource
	}
	if before, after, ok := strings.Cut(resource, "/"); ok {
		local := after
		if _, tail, found := strings.CutLast(after, "/"); found {
			local = tail
		}
		return before, local
	}
	if before, after, ok := strings.Cut(resource, ":"); ok {
		return before, after
	}
	return "", ""
}

func czrnType(arnType string) string {
	if arnType == "natgateway" {
		return "nat-gateway"
	}
	return strings.ReplaceAll(arnType, "_", "-")
}
