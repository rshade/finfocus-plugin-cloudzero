# Spec Delta

## ADDED Requirements

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
