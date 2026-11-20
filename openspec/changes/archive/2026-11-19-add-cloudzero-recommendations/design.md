# Design

## Context

Core asks every plugin that advertises `RECOMMENDATIONS` with one request per
batch of stack resources (`recommendationsRequest` in core's
`internal/engine/engine.go`). It sends `target_resources` (each carrying the
resource's URN as `id` and its properties, including `pulumi:cloudId`, as
tags), `projection_period: "monthly"`, `include_dismissed`, and the ids it
has dismissed locally in `excluded_recommendation_ids`. It follows
`next_page_token` for up to 100 pages with a 2-minute timeout per page.
Core attaches a recommendation to a resource when the recommendation's
resource id equals the resource's URN, and then tries the cloud id and ARN
(`fetchAndMergeRecommendations` in core's `internal/cli/common_execution.go`).
Core keeps dismissals in its own state file, so a plugin without
`DismissRecommendation` still supports dismissal.

The SDK (`finfocus-spec` v0.7.12) provides `ApplyRecommendationVisibility`,
`ApplyRecommendationFilter`, `PaginateRecommendations` (default 50, max
1000), and `CalculateRecommendationSummary`, and its server rejects a
response without a summary.

CloudZero's live response (2026-11-19, `initial-30` account) had 8
recommendations, all "Upgradable EBS Volumes" (`CIR-AWS-00010`), worth
$0.83 a month together. The `resource` field held CZRNs such as
`czrn:aws:ec2:us-west-2:<account>:volume:vol-...`, matching what
`mapping.Filter` builds. With the default `cost_impact_last_30_days[gte]=1.00`
the response was empty. The first call returned HTTP 504, and a retry
seconds later succeeded in under 2 seconds.

`mapping.Mapper.Filter` refuses to work without `CLOUDZERO_RESOURCE_DIMENSION`
because the costs query groups by it. Recommendations name their resource
directly and do not use that dimension.

## Goals / Non-Goals

**Goals:** CloudZero recommendations attached to the right stack resources,
from one CloudZero call per cache window, with the same budget, cooldown,
retry, and error mapping as the costs read.

**Non-Goals:** dismissing, snoozing, or updating recommendations in
CloudZero (Hard No 5); recommendation history; insights (`/v2/insights`);
mapping CloudZero recommendation types onto action types or rightsizing
details; Kubernetes `cluster`, `namespace`, and `pod` fields.

## Decisions

### One organization-wide fetch, scoped locally

The list is fetched once with no resource filter and matched to targets in
the plugin, as the costs read does (Hard No 3). A CloudZero filter per target
(`resource[eq]`) would spend one request per resource. The default page size
is 60,000, so one first page normally holds the whole list. Pages for core's
`page_token` come from the cached list, not from CloudZero.

### Matching reuses the cost identity, without the dimension

`Mapper` gains `Identity(resource) (Filter, error)`, which resolves the
`ResourceID` or `ResourceSuffix` and `Provider` exactly as `Filter` does today,
and `Filter` calls it, then adds the shared filters and the dimension check.
Core's recommendations request never sets `ResourceDescriptor.arn`. It
sends the ARN only as the `pulumi:arn` tag (`ingest/state.go`), so identity
also reads the `pulumi:arn` and `arn` tags, after the descriptor `arn` and
before `id`. The cost path already gets the same ARN through the legacy
`arn` field, so its results do not change. A recommendation matches a target
when `Filter.Matches(resource, cloudProvider)` holds for any of its cloud
providers. The bare-id suffix path
is what lets `vol-` ids from Pulumi state match without an ARN. Alternative:
match on `resource_name` against the cloud id. Rejected, because it only works
for AWS ids that happen to be names.

### The target's id is the resource id

When a recommendation matches a target, its `resource.id` is the target's
`id` (core's URN), which is the correlation the spec asks for and the first
key core matches on. Without targets it is the CZRN. A recommendation that
matches two targets, which needs two targets with the same CZRN, is returned
once per target.

### Floor at one cent, all statuses that can be shown

The query sets `cost_impact_last_30_days[gte]=0.01`, the lowest floor the
filter accepts (`exclusiveMin: 0`), and asks for `not_started`,
`in_progress`, and `ignored` as repeated `status` values, the array form the
reference page documents. The response metadata lists `eq` and `neq` for
`status`, so if CloudZero rejects the array form the fallback is a single
`status[neq]=addressed`. The live check on 2026-11-19 (task 5.2) found the
array form accepted. CloudZero's default floor of $1.00 hid every
recommendation on the owner's account. Core can apply a higher minimum
through `filter.min_estimated_savings`. `addressed` is never fetched. Whether
CloudZero filters statuses by default is not documented, so the plugin names
them. Alternative: a `CLOUDZERO_RECOMMENDATION_FLOOR` setting. Rejected until
someone needs it.

### Visibility through the SDK helper

`ignored` ids become `RecommendationVisibility.DismissedIDs`, so
`ApplyRecommendationVisibility` hides them unless `include_dismissed` is set,
and always drops `excluded_recommendation_ids`. Then `ApplyRecommendationFilter`,
`PaginateRecommendations`, and `CalculateRecommendationSummary` run in that
order. Writing these by hand would duplicate SDK behaviour that core's tests
already assume.

### No inferred action type or priority

CloudZero's recommendation types are listed by an API this plugin does not
read, and only one has been seen. Every recommendation gets action type `OTHER`, category `COST`, and
priority unspecified. CloudZero's type id and name, effort, status, and
`account` list go in metadata, and the title is the description. Effort is not
priority. A table of type ids would be an invented mapping (Hard No 4).

### Savings scale linearly from 30 days

`cost_impact_last_30_days` is taken as a monthly amount: daily is `/30`,
annual is `*365/30`. The field's name fixes the window, so no other reading
fits. Core asks for the monthly figure.

### The read spends budget like a costs first page

Same reasoning as `add-cloudzero-budgets`: no limit is documented for this
endpoint, the 60-a-day limit is shared, and under-counting risks a 429 for
every caller. Cached for `CLOUDZERO_RECENT_TTL` behind `inflight` (generic
since the budgets change), so a core run with many pages and batches costs
one unit. Core starts the plugin for each command, so each CLI run that asks
spends one unit. Core's own recommendations cache (on by default, 1h) skips
the plugin on repeat runs, except after a run with errors.

### Interfaces

`CloudZero` gains `RecommendationSource`
(`GetRecommendations(ctx) ([]czclient.Recommendation, error)`).
`*czclient.Client` implements it, and the test fake adds the one method.

## Risks / Trade-offs

- [Other recommendation types name a resource that is not a CZRN, or null]
  → rows without a resource are dropped, and rows whose resource matches no
  target are left out. Without targets they are still listed. Only one type
  has been seen live.
- [A large organization's list is big] → the client caps a body at 8 MiB
  (`maxBodyBytes`). A larger body fails as malformed rather than giving a
  partial list. Issue #14 measures large accounts.
- [HTTP 504 on a cold query] → the existing retry covers it. Each attempt
  can take about 30 seconds and spends budget, so the worst case is 3 units
  and about 90 seconds, inside core's 2-minute page timeout.
- [Sub-dollar recommendations are noise] → they are real and CloudZero shows
  them in its own console. Core's `min_estimated_savings` filters them.
- [Currency] → the same `CLOUDZERO_CURRENCY` assumption as cost results (#13).

## Migration Plan

Stacked on `add-cloudzero-budgets`. Its `plugin-info` delta modifies the
requirement that change creates, so this change archives second, after the
budgets change is archived. Rollback is a release without the capability.
