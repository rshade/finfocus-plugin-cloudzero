# Design

## Context

`mapping.Filter` identifies one resource by an exact CZRN (`ResourceID`) or a
CZRN suffix (`ResourceSuffix`), and `Filter.Matches` decides whether a
CloudZero row belongs to it. `rowsFor` turns matching rows into one
`ActualCostResult` per day with the source `cloudzero:<cost type> <currency>`.
Core (`totalActualCost` in `internal/proto/adapter.go`) sums every result into
the total and adds each to `breakdown[source]`. Its daily history sums
results that share a day (`history.PointsFromObservations`).

## Goals / Non-Goals

**Goals:** count an instance's root volume in its actual cost, visible as its
own breakdown line, with no extra CloudZero call.

**Non-Goals:** data volumes; network interfaces and Elastic IP charges (#15);
Terraform's `root_block_device` shape, which no caller sends today.

## Decisions

### The root volume is a second matcher on the same filter

`Filter` gains `RootVolumeSuffix`, `:volume:<vol-id>`, set from
`attributes.rootBlockDevice.volumeId` (an object, or a list whose first entry
has it) when the provider is AWS and the id starts with `vol-`.
`Filter.MatchesRootVolume` applies the same CloudProvider check as `Matches`.
A suffix is enough because the rows already come from the instance's account
and region, and a volume id is unique within them.

### Volume rows are separate results, not merged into the instance's

The owner chose a separate breakdown line (2026-11-18). `rowsFor` emits a
matching volume row as its own result with "root volume" appended to the
source. Merging into the day's cost would hide the split; lineage is an
allocation chain, not a list of parts. No protocol change is needed.

### Only `rootBlockDevice`

`ebsBlockDevices` and volume attachments are left out, as the issue requires:
a data volume can move between instances, and an attachment is its own
Pulumi resource.

## Risks / Trade-offs

- [A root volume also managed as its own Pulumi resource] → its cost would be
  counted on both. Pulumi does not create one for a root volume, so this needs
  a deliberate import; not handled.
- [A program that sets `rootBlockDevice` itself, such as a larger disk] → core
  builds attributes with a shallow merge in which inputs replace outputs
  (`ingest.MergeProperties`), so the input block drops the output-only
  `volumeId` and the volume is not added. The fix is a deep merge in core; the
  plugin needs no change once the id arrives.
- [A stale volume id after the root volume is replaced] → the old id simply
  matches no rows after the change; days before it still bill correctly.
