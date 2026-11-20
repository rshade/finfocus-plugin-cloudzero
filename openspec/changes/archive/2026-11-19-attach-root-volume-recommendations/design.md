# Design

## Context

`GetRecommendations` matches CloudZero's organization-wide list to core's
targets with `Mapper.Identity` and `Filter.Matches`, so a recommendation
attaches only to the target whose own CZRN it names. The cost path already
handles root volumes: `Mapper.Filter` reads
`attributes.rootBlockDevice.volumeId` into `Filter.RootVolumeSuffix`, and
`Filter.MatchesRootVolume` matches rows ending in `:volume:<id>`
(`include-root-volume-cost`). Recommendation targets carried no `attributes`
until rshade/finfocus#2281, which sends the same redacted attributes as
actual-cost descriptors. Its tags are capped at 50, which drops the dotted
`rootBlockDevice.volumeId` on an instance with 65 properties, so the
attributes are the only source.

## Goals / Non-Goals

**Goals:** a recommendation on an instance's root volume appears on that
instance, with no extra CloudZero call, and still says which disk it is
about.

**Non-Goals:** attaching data volumes (`ebsBlockDevices`), which the cost
path does not attach either; reading the volume id from core's flattened
tags; changing the cost path.

## Decisions

### Identity resolves the root volume

The root-volume suffix moves from `Filter` into the shared `identity` step, so
`Identity` and `Filter` both carry it. `Filter` behaves as before. The rule
stays AWS only, reads an object or a one-element list, and rejects ids that
are not `vol-`.

### A root volume match is a target match

`matchesTarget` accepts `Matches` or `MatchesRootVolume` for any of the
recommendation's cloud providers. The recommendation then carries the
target's `id`, as for a direct match. A volume that is also a stack resource
would match both its own target and its instance, and is returned once per
target, as the existing rule says for duplicate matches.

### The CZRN stays visible in metadata

Once a volume recommendation sits under an instance's URN, its resource id no
longer names the disk. Every recommendation gains `cloudzero_resource` with
CloudZero's CZRN. The resource type and name (`volume`, `vol-...`) already
come from CloudZero and stay as they are.

## Risks / Trade-offs

- [Suffix-only match on an organization-wide list] → the cost path's rows
  come from the instance's own account and region, while recommendations span
  the organization. AWS volume ids are 17 hexadecimal digits and unique in
  practice, so a false match needs an id collision across accounts.
- [Older core] → targets carry no attributes and nothing changes; the
  limitation documented in the README stays true for that core.
- [Core's shallow property merge] → when a program sets an input
  `rootBlockDevice` (disk size), `ingest.MergeProperties` can replace the
  output block that holds `volumeId`. The cost path has the same gap.

## Migration Plan

Stacked on `add-cloudzero-recommendations`, whose `recommendations` spec this
delta modifies, so this change archives second. It is verified live with a
core built from the #2281 branch. Rollback is a release without it; with no
attributes from core the behaviour is already the old one.
