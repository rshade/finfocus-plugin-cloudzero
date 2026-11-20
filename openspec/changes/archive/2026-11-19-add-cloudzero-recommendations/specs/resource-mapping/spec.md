# Spec Delta

## ADDED Requirements

### Requirement: Identity resolves without the resource dimension

Resolving a resource's CZRN or AWS suffix, and its provider, SHALL follow the
same order and rules as a cost filter, and SHALL NOT need
`CLOUDZERO_RESOURCE_DIMENSION`. Only building a cost filter needs it.

#### Scenario: No resource dimension

- **WHEN** no resource dimension is configured and the descriptor carries an EBS volume ARN
- **THEN** identity resolves to the volume's CZRN

Tested by: TestIdentityWithoutResourceDimension

## MODIFIED Requirements

### Requirement: Identity is resolved once, in a fixed order

The mapper SHALL try, in order, the descriptor's `pulumi:cloudId` tag, `arn`,
`pulumi:arn` tag, `arn` tag, and `id`, then the legacy request
`tags["pulumi:cloudId"]`, `arn`, and `resource_id`. The first identifier that
yields a CZRN SHALL identify the resource. An existing `czrn:` value SHALL be
used unchanged.

#### Scenario: An ARN in the cloudId tag is converted

- **WHEN** the descriptor carries an ARN only in its `pulumi:cloudId` tag
- **THEN** the resource id is the CZRN converted from that ARN

#### Scenario: The descriptor ARN beats a legacy czrn

- **WHEN** the descriptor has an ARN and the legacy resource id is a `czrn:` value
- **THEN** the resource id comes from the descriptor ARN

#### Scenario: The ARN arrives only as a tag

- **WHEN** the descriptor's `pulumi:cloudId` is `vol-1`, its `arn` field is empty, and its `pulumi:arn` tag is the volume's ARN
- **THEN** the resource id is the CZRN converted from that ARN

Tested by: TestFilterIdentityPrecedence, TestFilterLegacyFieldsWhenResourceMissing

### Requirement: Shared filters come only from known identifiers and configuration

Shared filters SHALL be `Account` (AWS account, Azure subscription, or GCP
project), `Region` (descriptor region, else the ARN region), and one entry per
configured `tagKey=dimensionID` pair whose tag is present. No dimension ID
SHALL be inferred from a tag name. Without a configured resource dimension,
building a cost filter SHALL fail.

#### Scenario: Configured tag dimension

- **WHEN** `Team` maps to `User:Defined:Team` and the resource has `Team=platform` and `cost_center=nope`
- **THEN** the filters include `User:Defined:Team=platform` and nothing for `cost_center`

Tested by: TestFilterCloudIdentifiers, TestFilterRejectsMissingIdentity
