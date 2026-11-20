# `finfocus-plugin-cloudzero` Project Context

This document defines the technical guardrails and architectural scope of
`finfocus-plugin-cloudzero`. ROADMAP.md, OpenSpec changes, and new features
must stay inside these boundaries.

## Core Architectural Identity

**Live/Runtime gRPC read-only plugin for what CloudZero has billed and
recorded.**

`finfocus-plugin-cloudzero` is a FinFocus cost source that reads what a cloud
resource has already cost, per day, from the CloudZero billing costs API. It
answers `GetActualCost` and actual-cost `BatchCost` for AWS, Azure, and GCP
resources, `GetBudgets` from the budgets CloudZero keeps, and
`GetRecommendations` from the optimization recommendations CloudZero has made.
It is a reader of CloudZero's records, not a pricing engine: it never predicts, estimates, or
prices anything CloudZero has not billed.

## Technical Boundaries ("Hard No's")

1. **No projected cost, pricing spec, or estimate.** These RPCs return
   `Unimplemented`. The plugin never returns `$0` for them, because core treats
   a `$0` projection as a real price and will not fall back to another plugin.
2. **No slice cost attributed to a resource.** Without a resource-level
   dimension (`CLOUDZERO_RESOURCE_DIMENSION`) the plugin declines. It never
   returns an account, service, or tag slice as a resource's cost, because that
   slice would be counted once per resource in a stack total.
3. **No per-resource queries.** CloudZero allows 60 cost requests in a rolling
   24 hours, shared with every other caller of the key or organization. The
   query is month-aligned and filters only on shared dimensions (`Account`,
   `Region`, configured tag dimensions), so one call serves many resources. It
   never filters on the resource or on `CloudProvider`, and it never loops on
   HTTP 429.
4. **No invented dimension IDs or spellings.** Tag filters come only from
   configured `tagKey=dimensionID` pairs. `CloudProvider` spellings are read
   from CloudZero's own rows and never hardcoded.
5. **No writes to CloudZero.** The plugin uses the read-only scopes
   `billing:read_costs` and `billing:read_dimensions`, and reads budgets and
   recommendations with the same key. It never sends AnyCost data, telemetry,
   or CostFormation changes, never creates or edits a budget, and never
   dismisses or updates a recommendation. Core keeps dismissals itself.
6. **No infrastructure access.** The plugin does not call AWS, Azure, or GCP,
   and never checks whether a resource exists. Its only inputs are the
   `ResourceDescriptor` from core and CloudZero's response.
7. **No persistent storage.** The cache is in memory and dies with the process.
   Core owns long-term caching through `expires_at`.
8. **No secrets in output.** The API key never appears in a log line, an error
   message, or a URL. Upstream error bodies are never forwarded to core.
9. **No unattended live calls.** CI and agent sessions use fakes and the
   sanitized fixtures in `testdata/live`. A live CloudZero call needs the
   owner's go-ahead, because every call spends the shared daily budget.

## Data Source of Truth

- **Cost**: CloudZero `GET /v2/billing/costs`, grouped by the resource
  dimension and `CloudProvider`, at daily granularity. The default `cost_type`
  is `real_cost`, CloudZero's own default.
- **Resource identity**: a CloudZero resource name (CZRN). An AWS ARN is
  converted. A bare AWS id with a known prefix matches by CZRN suffix. Azure
  and GCP resources need a `czrn:` id from the caller.
- **Budgets**: CloudZero `GET /v2/budgets` with `expand=current`. Monthly
  budgets only. The limit is the current month's planned amount, and the status
  is CloudZero's actual. Core derives health and forecast.
- **Recommendations**: CloudZero `GET /v2/optimize/recommendations`, every
  status but `addressed`, worth at least $0.01 over the last 30 days. Each
  names its resource as a CZRN and is matched to core's target resources with
  the same identity rules as cost rows. Savings are the 30-day cost impact,
  scaled to the requested period. `ignored` counts as dismissed.
- **Currency**: not in the response. `CLOUDZERO_CURRENCY` labels results.
- **Freshness**: CloudZero sends no TTL. The plugin computes `expires_at`: a
  short TTL for the newest two days, `CLOUDZERO_CACHE_TTL` for older days.

## Interaction Model

- **Protocol**: gRPC, implementing `finfocus.v1.CostSourceService`.
- **Lifecycle**: started as a subprocess by FinFocus core.
- **Discovery**: announces its port on `stdout` as `PORT=XXXXX`. Nothing else
  is written to `stdout`.
- **Telemetry**: structured JSON logs (`zerolog`) on `stderr`.
- **Network strategy**: fetch and cache.
  - **Fetch**: one month-aligned query per shared filter set, with cursor
    paging. Cursor pages do not count against the CloudZero limit. Budgets and
    recommendations are each one list query, cached for
    `CLOUDZERO_RECENT_TTL` and matched to resources locally.
  - **Budget**: a per-process counter stops before `CLOUDZERO_DAILY_BUDGET`
    (default 50). Every first-page attempt counts, retries included.
  - **Resilience**: a short retry for transport errors and HTTP 5xx, one
    restart on HTTP 410, and no retry on HTTP 429.

## Key Technologies

- **Language**: Go 1.27.4
- **Spec**: `github.com/rshade/finfocus-spec` v0.7.12
- **Protocol**: `google.golang.org/grpc`
- **Logging**: `github.com/rs/zerolog`
- **Release**: Release Please and GoReleaser, six archives plus `checksums.txt`

## Change Process

- **OpenSpec**: behavior changes go through an OpenSpec change (propose,
  apply, verify, archive). `openspec/specs/` holds current behavior.
- **Direct fixes**: tests, docs, CI, and a fix that changes no requirement
  skip OpenSpec.
- **Commits**: Conventional Commits. Release Please owns `CHANGELOG.md` and the
  release manifest.

## Verification

Before accepting a feature, check it against these questions. A "yes" means
the feature crosses a boundary and needs a different design.

1. Does it return a cost CloudZero has not billed, or a `$0` placeholder?
2. Does it spend one CloudZero request per resource, or retry a 429?
3. Does it hardcode a CloudZero dimension ID, value, or spelling?
4. Does it write to CloudZero or to a cloud provider?
5. Does it persist anything beyond the process lifetime?
6. Could it put the API key, a query string, or an upstream body into output?
7. Does a test or CI job call the live API?
