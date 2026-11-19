# Proposal

## Why

CloudZero bills an EC2 instance's root EBS volume as its own `volume`
resource. The root volume is not a Pulumi resource, so no stack resource
carries its cost and the stack total leaves it out. On the `initial-30` `dev`
stack (2026-10-17 to 2026-11-17) that was five root volumes at about $0.80 a
month each and a Windows root volume at about $2.99. Pulumi state names the
root volume on the instance (`rootBlockDevice.volumeId`), and core sends it in
`ResourceDescriptor.attributes`. Issue #10.

## What Changes

- An AWS resource whose attributes name `rootBlockDevice.volumeId` also
  matches CloudZero rows whose CZRN ends in `:volume:<that id>`.
- Those rows come back as their own daily results with the source
  `cloudzero:<cost type> <currency> root volume`. Core adds every result into
  the resource's total and keys its breakdown by source, so the instance's
  total includes the volume and the breakdown shows it as a separate line.
- The volume rows are already in the month query the instance uses (same
  account and region), so there is no extra CloudZero call.
- Data volumes (`ebsBlockDevices`, `aws:ec2/volumeAttachment`) are not added.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `resource-mapping`: a resource's filter can carry its root volume.
- `actual-cost`: root volume rows are reported as a separate source.

## Impact

- Code: `internal/mapping/mapping.go` (`Filter.RootVolumeSuffix`, read from
  attributes), `internal/plugin/actual.go` (`rowsFor`), and tests.
  `GetActualCost` and `BatchCost` both go through `rowsFor`.
- Behaviour: instance totals rise by their root volume's billed cost. No
  change to the query, the call count, configuration, or the gRPC contract.
- CONTEXT.md Hard No 2 (no slice cost on a resource) holds: the root volume is
  part of the instance and is billed for it.
