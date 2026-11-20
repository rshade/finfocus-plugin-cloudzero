# Proposal

## Why

FinFocus core shows optimization recommendations next to each stack resource
(`finfocus cost recommendations`, and the detail view of `cost actual`) from
every plugin that serves `GetRecommendations`. CloudZero produces
recommendations (`GET /v2/optimize/recommendations`), but this plugin serves
none. A live call on 2026-11-19 showed that each recommendation names its
resource as a CZRN, the same identifier the plugin already matches cost rows
on, so they can be tied to stack resources without guessing. The existing
read key works (HTTP 200).

## What Changes

- The plugin serves `GetRecommendations`. One query for the whole
  organization, with cursor paging, fetches every open and ignored
  recommendation worth at least $0.01 over the last 30 days. CloudZero's
  default floor of $1.00 hid all 8 recommendations on the owner's account.
- With `target_resources`, only recommendations whose CZRN matches a target
  are returned, each with the target's `id`, so core attaches it to that
  resource. Without targets, every recommendation is returned under its CZRN.
- Each recommendation keeps CloudZero's id, title, and type. Its savings are
  CloudZero's 30-day cost impact, scaled to the requested projection period,
  in `CLOUDZERO_CURRENCY`.
- `ignored` recommendations count as dismissed: returned only when core sets
  `include_dismissed`. `addressed` recommendations are never returned.
- The SDK helpers apply core's filter, excluded ids, paging, and summary.
- `DismissRecommendation` and `GetRecommendationHistory` stay unimplemented.
  Core keeps dismissals locally, and the plugin never writes to CloudZero.
- `GetPluginInfo` adds `RECOMMENDATIONS`. `Supports` accepts it with only the
  API key set.
- The list is cached for `CLOUDZERO_RECENT_TTL`. The first page spends one
  unit of `CLOUDZERO_DAILY_BUDGET` and respects the rate-limit cooldown.
- Resource identity (`pulumi:cloudId`, ARN, id, to a CZRN or an AWS suffix)
  no longer requires `CLOUDZERO_RESOURCE_DIMENSION`, which only the costs
  query needs.

## Capabilities

### New Capabilities

- `recommendations`: CloudZero optimization recommendations scoped to the
  resources core asks about.

### Modified Capabilities

- `plugin-info`: the declared capabilities add `RECOMMENDATIONS`, and
  `Supports` accepts it without a resource dimension.
- `cloudzero-client`: a recommendations read joins the costs, dimensions,
  and budgets reads.
- `resource-mapping`: resolving a resource's CZRN does not need the resource
  dimension.

## Impact

- Depends on `add-cloudzero-budgets`, which renames the capabilities
  requirement, adds `BUDGETS`, and makes `inflight` generic. The two land as
  stacked PRs, budgets first.
- Code: `internal/czclient` (recommendations read and parse),
  `internal/mapping` (identity without the dimension), `internal/plugin`
  (`recommendations.go`, `RecommendationSource`, `Info`, `Supports`), tests,
  and a sanitized live fixture of the 2026-11-19 response.
- Behaviour: core shows CloudZero recommendations per resource. Nothing
  changes for actual cost or budgets.
- Rate limit: one extra first-page call per `CLOUDZERO_RECENT_TTL` window
  while core asks for recommendations.
- Docs: `CONTEXT.md` (Data Source of Truth), `README.md`, `docs/design.md`,
  `CLAUDE.md`.
