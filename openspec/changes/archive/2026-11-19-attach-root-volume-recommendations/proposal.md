# Proposal

## Why

CloudZero recommends on an EC2 instance's root EBS volume as its own `volume`
CZRN, and Pulumi does not manage root volumes as resources, so those
recommendations match no target and core shows nothing. On 2026-11-19 the
`initial-30` `dev` stack (six instances, no volume resources) sent 31 targets
and got 0 of CloudZero's 8 recommendations, all on root volumes (#29). Core
now sends each target's nested properties as `attributes`
(rshade/finfocus#2281), which carry `rootBlockDevice.volumeId`.

## What Changes

- A recommendation whose resource is a target's root volume, named by the
  target's `attributes.rootBlockDevice.volumeId`, is returned for that target
  under the target's `id`, as actual cost already adds the root volume's rows
  to its instance.
- Data volumes (`ebsBlockDevices`) are still not attached, matching the cost
  path.
- Every recommendation keeps CloudZero's resource CZRN in metadata as
  `cloudzero_resource`, so a recommendation shown on an instance still says
  it is about the disk.
- No extra CloudZero call: matching runs on the cached list.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `recommendations`: a target's root volume matches as well as the target
  itself, and metadata adds the resource CZRN. The capability is created by
  `add-cloudzero-recommendations`, which archives first.

## Impact

- Code: `internal/mapping` (identity also resolves the root volume),
  `internal/plugin/recommendations.go` (matching and metadata), and tests.
- Depends on core sending `attributes` on recommendation targets
  (rshade/finfocus#2281). With an older core the targets carry no attributes,
  and behaviour is unchanged.
- Docs: README "Recommendations" drops the root-volume limitation,
  docs/design.md, CLAUDE.md.
- Closes #29.
