# resource-mapping Specification

## Purpose

Turn a FinFocus resource descriptor into a CloudZero resource name and shared
dimension filters.

## Requirements

### Requirement: Identity is resolved once, in a fixed order

The mapper SHALL try, in order, the descriptor's `pulumi:cloudId` tag, `arn`,
and `id`, then the legacy request `tags["pulumi:cloudId"]`, `arn`, and
`resource_id`. The first identifier that yields a CZRN SHALL identify the
resource. An existing `czrn:` value SHALL be used unchanged.

#### Scenario: An ARN in the cloudId tag is converted

- **WHEN** the descriptor carries an ARN only in its `pulumi:cloudId` tag
- **THEN** the resource id is the CZRN converted from that ARN

#### Scenario: The descriptor ARN beats a legacy czrn

- **WHEN** the descriptor has an ARN and the legacy resource id is a `czrn:` value
- **THEN** the resource id comes from the descriptor ARN

Tested by: TestFilterIdentityPrecedence, TestFilterLegacyFieldsWhenResourceMissing

### Requirement: AWS ARNs convert to CZRNs

An AWS ARN SHALL convert to `czrn:<provider>:<service>:<region>:<account>:<type>:<id>`.
`natgateway` SHALL become `nat-gateway`. An empty region SHALL use the
descriptor region, else `cross-region`. The `aws-cn` and `aws-us-gov`
partitions SHALL map to `aws`. An ARN without an account or resource SHALL
not convert.

#### Scenario: NAT gateway ARN

- **WHEN** the ARN is `arn:aws:ec2:us-west-2:123456789012:natgateway/nat-abc`
- **THEN** the CZRN is `czrn:aws:ec2:us-west-2:123456789012:nat-gateway:nat-abc`

Tested by: TestFilterCloudIdentifiers, TestFilterIdentityPrecedence, TestFilterRejectsMissingIdentity

### Requirement: Bare AWS ids match by CZRN suffix

When no identifier yields a CZRN, an AWS resource with a region and a bare id
prefixed `i-`, `vol-`, `eni-`, `nat-`, or `eipalloc-` SHALL match rows ending
in `:<type>:<id>`. Its shared filters SHALL have no `Account`. Any other prefix,
a missing region, or a non-AWS provider SHALL be declined.

#### Scenario: NAT gateway without an ARN

- **WHEN** an AWS resource in `us-west-2` has only the id `nat-0123456789abcdef0`
- **THEN** it matches the suffix `:nat-gateway:nat-0123456789abcdef0` and filters on `Region` only

Tested by: TestFilterMatchesBareAWSIDBySuffix, TestFilterRejectsBareIDWithoutContext

### Requirement: Provider matching uses CloudZero's own spelling

The provider SHALL be normalized to `aws`, `azure`, or `gcp` (`-native` is
dropped, `azurerm` becomes `azure`, `google` becomes `gcp`). A row SHALL match
only when its `CloudProvider` value equals that provider, ignoring case. No
CloudZero spelling SHALL be hardcoded.

#### Scenario: Row from another provider

- **WHEN** a filter for an `aws` resource meets a row whose `CloudProvider` is `Azure`
- **THEN** the row does not match

#### Scenario: Pulumi provider aliases

- **WHEN** the provider is `google-native` or `azurerm`
- **THEN** it is supported

Tested by: TestFilterMatches, TestSupportedProvider

### Requirement: Shared filters come only from known identifiers and configuration

Shared filters SHALL be `Account` (AWS account, Azure subscription, or GCP
project), `Region` (descriptor region, else the ARN region), and one entry per
configured `tagKey=dimensionID` pair whose tag is present. No dimension ID
SHALL be inferred from a tag name. Without a configured resource dimension the
mapper SHALL fail.

#### Scenario: Configured tag dimension

- **WHEN** `Team` maps to `User:Defined:Team` and the resource has `Team=platform` and `cost_center=nope`
- **THEN** the filters include `User:Defined:Team=platform` and nothing for `cost_center`

Tested by: TestFilterCloudIdentifiers, TestFilterRejectsMissingIdentity

### Requirement: An instance's filter carries its root volume

When an AWS resource's attributes name `rootBlockDevice.volumeId` with a `vol-`
id, its filter SHALL also match rows whose resource value ends in
`:volume:<that id>` and whose CloudProvider matches. Data volumes SHALL NOT be
matched, and a resource without that attribute SHALL be unchanged.

#### Scenario: An instance with a root volume

- **WHEN** an instance's attributes carry `rootBlockDevice.volumeId` `vol-1`
- **THEN** its filter matches `czrn:aws:ec2:us-east-1:123456789012:volume:vol-1` as its root volume

#### Scenario: Volumes that are not the root volume

- **WHEN** an instance lists `vol-2` in `ebsBlockDevices`, or names no root volume
- **THEN** its filter matches no volume rows

Tested by: TestFilterRootVolume
