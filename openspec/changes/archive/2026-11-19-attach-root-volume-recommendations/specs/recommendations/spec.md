# Spec Delta

## ADDED Requirements

### Requirement: A target's root volume recommendations attach to it

When an AWS target's attributes name `rootBlockDevice.volumeId` with a `vol-`
id, a recommendation whose resource ends in `:volume:<that id>` and whose
cloud provider matches SHALL be returned for that target, under the target's
`id`. Data volumes SHALL NOT be attached, and a target without that attribute
SHALL match only its own CZRN. Matching SHALL make no extra CloudZero call.

#### Scenario: An instance and its root volume

- **WHEN** a target's attributes carry `rootBlockDevice.volumeId` `vol-1` and CloudZero recommends on `czrn:aws:ec2:us-west-2:123456789012:volume:vol-1`
- **THEN** that recommendation is returned with the target's id as its resource id

#### Scenario: Volumes that are not the root volume

- **WHEN** a target lists `vol-2` only in `ebsBlockDevices`, or its attributes name no root volume
- **THEN** a recommendation on `vol-2` is not returned for it

#### Scenario: Core sends no attributes

- **WHEN** a target carries no attributes
- **THEN** it receives only recommendations on its own CZRN

Tested by: TestGetRecommendationsAttachesRootVolume

## MODIFIED Requirements

### Requirement: A recommendation keeps CloudZero's identity and words

Each recommendation SHALL carry CloudZero's `recommendation_id` as its id,
source `cloudzero`, category cost, action type other, CloudZero's title as
its description, and its `created` time. Its resource SHALL carry the
provider, CloudZero's resource type, the first region, and the resource
name. CloudZero's type id, type name, effort, status, accounts, and resource
CZRN SHALL be kept in metadata. No action type or priority SHALL be inferred.

#### Scenario: Fields from the live response

- **WHEN** CloudZero returns an "Upgradable EBS Volumes" recommendation of type `CIR-AWS-00010` with effort low
- **THEN** the description is "Upgradable EBS Volumes", the action type is other, the priority is unspecified, and the metadata holds the type id, effort, and resource CZRN

Tested by: TestGetRecommendationsMapsFields
