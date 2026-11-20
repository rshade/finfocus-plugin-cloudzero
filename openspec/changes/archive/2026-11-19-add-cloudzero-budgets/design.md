# Design

## Context

The plugin embeds `pbc.UnimplementedCostSourceServiceServer`, so the SDK
server already finds a `BudgetsProvider` and gets `Unimplemented` back from
it. Core asks only plugins that advertise `BUDGETS`
(`collectPluginBudgets` in core's `internal/engine/budget.go`). It sends an
empty `GetBudgetsRequest`, so `include_status` is always false. Then
`prepareBudget` validates each budget (`ValidateBudget`: a non-empty id and a
limit that is not negative; a failing budget is skipped on its own), applies
default thresholds when none are given, and derives percentage used,
forecast, health, and triggered thresholds from `status.current_spend`.
Without a status, or with a zero limit, there is no health. Core's provider
filter (`FilterBudgetsByProvider`) compares against `Budget.Source`.

CloudZero's `GET /v2/budgets` (docs `getbudgets.md`, fetched 2026-11-19)
returns `budgets[]` with `id`, `name`, `view.id`, `planned_limits.monthly` (a
map from a date to `{"amount"}`), `alerts` (a map such as `{"80": "Y"}`),
`cost_type`, and `granularity`. With `expand=current` each budget also has
`current: {planned, actual}`, documented as strings. Paging is the cursor
shape the costs read uses. The read key got HTTP 200 on 2026-11-19 with an
empty list. No budget has been seen live.

The client counts every first-page attempt against `CLOUDZERO_DAILY_BUDGET`
and fails it during the cooldown after a 429 (`admit`). The plugin shares
one in-flight fetch per cache key (`inflight`) and caches results in memory.

## Goals / Non-Goals

**Goals:** list CloudZero budgets with spend, from one call per cache
window, through the same budget, cooldown, retry, and error mapping as the
costs read.

**Non-Goals:** creating, editing, or deleting budgets (Hard No 5); the
`history` and `burnup` expansions; quarterly or annual budgets beyond what
the monthly limits give; mapping a CloudZero view to core's provider, region,
and tag filter.

## Decisions

### Status is always sent, whatever `include_status` says

Core never sets `include_status`, and without a status it computes no health,
so honouring the flag would make every CloudZero budget useless.
`expand=current` comes in the same call, so the status costs nothing extra.
Alternative: honour the flag and change core first. Rejected, because it
blocks the feature on a core release for no saving.

### The plugin reports spend and leaves the arithmetic to core

The status carries `current_spend` and `currency` only. Core recomputes
percentage, forecast, and health anyway. Filling them in the plugin would
duplicate core's rules and put a forecast in this plugin (Hard No 1).
CloudZero's `current.planned` is not used. The limit comes from
`planned_limits.monthly`, so limit and spend refer to the same month.

### The limit is the current UTC month's planned amount

`planned_limits.monthly` is keyed by date. The plugin takes the entry whose
key falls in the current UTC month. The docs do not fix the key format, so
the parser accepts `YYYY-MM-DD` and `YYYY-MM`. A budget with no such entry,
or a zero amount, is left out: core would list it with no health and a limit
of zero, which reads as a real budget of nothing.

### Only monthly budgets

A budget whose `granularity` is not `monthly` is left out. Its
`current.actual` may cover a quarter or a year, and core would compare that
with one month's planned amount and report it exceeded. The docs show only
`monthly`, so a budget with no `granularity` counts as monthly rather than
vanishing. Every budget left out is counted in a debug log line by reason, so
an empty list can be told apart from "no budgets".

### A view becomes metadata, not a filter

A CloudZero budget covers a CloudZero view, which has no counterpart in
`BudgetFilter` (providers, regions, resource types, tags). The view id goes
in `metadata["cloudzero_view_id"]` and the filter stays empty. Inventing
provider values from a view would break Hard No 4.

### The read spends budget like a costs first page

The 60-a-day limit is documented for costs only, and no limit is documented
for budgets. The client still charges the first page and applies the
cooldown, as it already does for dimensions. Under-counting risks a 429 for
every caller of the key. Core starts the plugin for each command, and the
cache and counter live in the process, so each CLI run that asks for budgets
spends one unit, as each `cost actual` run already spends at least one.
Alternatives: a separate counter (rejected until a live 429 shows the
endpoint has its own limit), or an opt-in setting (rejected: one unit a run
is the cost of the feature, and `CLOUDZERO_DAILY_BUDGET` already caps it).

### One cached fetch, shared by concurrent callers

Budgets are cached under one key for `CLOUDZERO_RECENT_TTL` (default 1h),
because the month's actual changes during the day. The cache helps a
long-running process. A CLI run starts empty. `inflight` becomes
generic over its result type, so budgets reuse its detached, deadline-keeping
fetch instead of copying it.

### Interfaces stay small

`plugin.New` takes a `CloudZero` interface that embeds `CostSource` and a new
`BudgetSource` (`GetBudgets(ctx) ([]czclient.Budget, error)`).
`*czclient.Client` implements both, and test fakes add the one method.

## Risks / Trade-offs

- [The response differs from the docs, for example a different date key or
  numeric amounts] → the parser accepts both string and number amounts and
  both key forms. A malformed body maps to `Unavailable` with a fixed
  message (`statusFrom`), never a wrong budget. The first owner with a budget checks it live.
- [A budget's currency differs from `CLOUDZERO_CURRENCY`] → the same
  assumption the cost results make. Issue #13 covers non-USD organizations.
- [Budgets counted against the shared limit] → one first-page call per CLI
  run that asks for budgets. The overview asks only when budgets are on.
- [A user filters budgets by provider, such as `aws`] → core compares the
  provider with `Budget.Source`, so CloudZero budgets are hidden. The source
  stays `cloudzero`: a CloudZero budget can span providers.
- [Error text names the cost query] → the bad-request and rate-limit texts
  say "cost query" and "cost requests". Task 1.3 makes them name the request
  without saying costs.

## Migration Plan

None. A new capability, and core asks for it only after the plugin
advertises it. Rollback is a release without the capability.
