# recommendations Specification

## Purpose

Return the optimization recommendations CloudZero has made for the resources
core asks about, tied to those resources by CZRN.

## Requirements

### Requirement: Recommendations are scoped to the target resources

With `target_resources`, `GetRecommendations` SHALL return only
recommendations whose resource matches a target's CZRN, exactly or by AWS
suffix, and whose cloud provider matches it ignoring case. Each SHALL carry
that target's `id` as its resource id. A target with no CZRN SHALL be
skipped. Without targets, every recommendation SHALL be returned with its CZRN
as its resource id.

#### Scenario: A target matched by CZRN

- **WHEN** a target's `pulumi:arn` tag converts to the CZRN of a volume recommendation, and the
  target id is a Pulumi URN
- **THEN** the response holds that recommendation with the URN as its resource id

#### Scenario: A bare AWS id

- **WHEN** a target's `pulumi:cloudId` is `vol-1` with region `us-west-2`, and a recommendation's
  resource ends in `:volume:vol-1`
- **THEN** that recommendation is returned for the target

#### Scenario: Recommendations for other resources

- **WHEN** CloudZero has recommendations for resources that no target names
- **THEN** none of them is returned

#### Scenario: No targets

- **WHEN** the request has no `target_resources`
- **THEN** every open recommendation is returned with its CZRN as its resource id

Tested by: TestGetRecommendationsScopesToTargets, TestGetRecommendationsWithoutTargets

### Requirement: A recommendation keeps CloudZero's identity and words

Each recommendation SHALL carry CloudZero's `recommendation_id` as its id,
source `cloudzero`, category cost, action type other, CloudZero's title as
its description, and its `created` time. Its resource SHALL carry the
provider, CloudZero's resource type, the first region, and the resource
name. CloudZero's type id, type name, effort, status, accounts, and resource
CZRN SHALL be kept in metadata. No action type or priority SHALL be inferred.

#### Scenario: Fields from the live response

- **WHEN** CloudZero returns an "Upgradable EBS Volumes" recommendation of type `CIR-AWS-00010` with effort low
- **THEN** the description is "Upgradable EBS Volumes", the action type is other, the priority is
  unspecified, and the metadata holds the type id, effort, and resource CZRN

Tested by: TestGetRecommendationsMapsFields

### Requirement: Savings are the 30-day cost impact, scaled to the period

Estimated savings SHALL be CloudZero's `cost_impact_last_30_days`, taken as
monthly, in `CLOUDZERO_CURRENCY`. The projection period SHALL be the
request's, default `monthly`. Daily SHALL divide by 30 and annual SHALL
multiply by 365/30. No current or projected cost SHALL be invented.

#### Scenario: Annual projection

- **WHEN** a recommendation's cost impact is 0.30 and the request asks for `annual`
- **THEN** its estimated savings are 3.65 with projection period `annual`

Tested by: TestGetRecommendationsScalesSavings

### Requirement: CloudZero status decides visibility

Recommendations that are `not_started` or `in_progress` SHALL be returned.
`ignored` SHALL count as dismissed and SHALL be returned only when
`include_dismissed` is set. `addressed` SHALL never be returned. IDs in
`excluded_recommendation_ids` SHALL never be returned.

#### Scenario: An ignored recommendation

- **WHEN** CloudZero marks a recommendation `ignored`
- **THEN** it is left out unless `include_dismissed` is set

#### Scenario: An excluded id

- **WHEN** core excludes a recommendation's id and sets `include_dismissed`
- **THEN** that recommendation is left out

Tested by: TestGetRecommendationsVisibility

### Requirement: Filters, paging, and summary follow the SDK

The request's `filter` SHALL be applied after scoping. Results SHALL be paged
by `page_size` and `page_token`, and the summary SHALL cover the returned
page, as the SDK helpers define them.

#### Scenario: Two pages

- **WHEN** five recommendations match and `page_size` is 3
- **THEN** the first response has three and a next page token, and the second has two and none

Tested by: TestGetRecommendationsPagesAndFilters

### Requirement: Recommendations are read once per cache window

Recommendations SHALL be fetched with one CloudZero query, cursor pages
included, and cached for `CLOUDZERO_RECENT_TTL`. Concurrent requests and
later pages SHALL share it. A failed fetch SHALL NOT be cached and SHALL map
to the same status codes as cost queries. A missing API key SHALL fail with
`FailedPrecondition` without a call.

#### Scenario: Paging through a cached list

- **WHEN** core reads three pages within `CLOUDZERO_RECENT_TTL`
- **THEN** CloudZero is called once

#### Scenario: CloudZero rate limit

- **WHEN** the recommendations query gets HTTP 429
- **THEN** `GetRecommendations` returns `ResourceExhausted` and nothing is cached

Tested by: TestGetRecommendationsCachesOneFetch, TestGetRecommendationsErrors

### Requirement: Dismissal and history are not served

`DismissRecommendation` and `GetRecommendationHistory` SHALL return
`Unimplemented`. The plugin SHALL NOT change a recommendation in CloudZero.

#### Scenario: Core dismisses a recommendation

- **WHEN** core calls `DismissRecommendation`
- **THEN** the status code is `Unimplemented` and no CloudZero call is made

Tested by: TestRecommendationWritesUnimplemented

### Requirement: A target's root volume recommendations attach to it

When an AWS target's attributes name `rootBlockDevice.volumeId` with a `vol-`
id, a recommendation whose resource ends in `:volume:<that id>` and whose
cloud provider matches SHALL be returned for that target, under the target's
`id`. Data volumes SHALL NOT be attached, and a target without that attribute
SHALL match only its own CZRN. Matching SHALL make no extra CloudZero call.

#### Scenario: An instance and its root volume

- **WHEN** a target's attributes carry `rootBlockDevice.volumeId` `vol-1` and CloudZero recommends
  on `czrn:aws:ec2:us-west-2:123456789012:volume:vol-1`
- **THEN** that recommendation is returned with the target's id as its resource id

#### Scenario: Volumes that are not the root volume

- **WHEN** a target lists `vol-2` only in `ebsBlockDevices`, or its attributes name no root volume
- **THEN** a recommendation on `vol-2` is not returned for it

#### Scenario: Core sends no attributes

- **WHEN** a target carries no attributes
- **THEN** it receives only recommendations on its own CZRN

Tested by: TestGetRecommendationsAttachesRootVolume
