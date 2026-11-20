# Proposal

## Why

FinFocus core can show budget health (`finfocus cost budget`, the overview's
budget panel) for every plugin that serves `GetBudgets`. CloudZero keeps
budgets with a monthly planned amount and the month's actual spend
(`GET /v2/budgets`), but this plugin answers only actual cost, so a CloudZero
user sees none of them. The read is one call per run, needs no new key scope
(the existing read key got HTTP 200 on 2026-11-19), and writes nothing.

## What Changes

- The plugin serves `GetBudgets`. One `GET /v2/budgets?expand=current`, with
  cursor paging, lists every CloudZero budget. Each becomes a FinFocus
  `Budget`: CloudZero id and name, source `cloudzero`, the current month's
  planned amount as a monthly limit in `CLOUDZERO_CURRENCY`, the alert
  percentages as actual-spend thresholds, and the month's actual spend as
  status.
- The plugin reports spend only. Core computes percentage used, health,
  forecast, and triggered thresholds (`prepareBudget`), so the plugin never
  forecasts.
- `GetPluginInfo` adds the `BUDGETS` capability, and `Supports` accepts it
  with only the API key set.
- The list is cached for `CLOUDZERO_RECENT_TTL`. The first-page call spends
  one unit of `CLOUDZERO_DAILY_BUDGET` and respects the rate-limit cooldown.
- `CONTEXT.md` changes the plugin's identity from an actual-cost plugin to a
  read-only plugin for what CloudZero has billed and recorded. The nine Hard
  No's are unchanged.

## Capabilities

### New Capabilities

- `budgets`: listing CloudZero budgets as FinFocus budgets with current spend.

### Modified Capabilities

- `plugin-info`: the declared capabilities add `BUDGETS`, and `Supports`
  accepts it without a resource dimension.
- `cloudzero-client`: a budgets read joins the costs and dimensions reads.

## Impact

- Code: `internal/czclient` (budgets read and parse), `internal/plugin`
  (`budgets.go`, the `BudgetSource` interface, `Info`, `Supports`), tests,
  and a docs-shaped fixture under `internal/czclient/testdata`.
- Behaviour: core lists CloudZero budgets. Nothing changes for actual cost.
- Rate limit: one extra first-page call per `CLOUDZERO_RECENT_TTL` window
  while core asks for budgets.
- Docs: `CONTEXT.md`, `README.md`, `docs/design.md`, `CLAUDE.md`.
- Unverified: no budget has been seen live, because the owner's organization
  has none. The fixture follows the documented schema.
