# CloudZero Plugin Design

This document describes the architectural design of the FinFocus CloudZero plugin,
including decisions around caching, dimension mapping, and batch processing.

## Overview

The CloudZero plugin is an **actual-cost** data source that queries the CloudZero
REST API v2 to retrieve historical billing data. It integrates with FinFocus core to
enable cost analysis, drift detection, and multi-cloud cost optimization.

**Key Characteristics:**

- Actual-cost only. Projected cost, pricing spec, and estimate return `Unimplemented`
- Multi-cloud support (AWS, Azure, GCP)
- Filtering by CloudZero dimension IDs (`Account`, `Region`,
  and configured tag dimensions)
- `expires_at` computed by the plugin. CloudZero sends no TTL
- Month-aligned queries shared across resources, so one call serves many

## Prerequisites

This design pins `finfocus-spec` v0.7.12, the version core uses.

### 1. `expires_at` Field on Cost Results

`ActualCostResult.expires_at` tells core when a row goes stale. Core caps the
TTL it derives from that hint.

### 2. BatchCost RPC

`CostSourceService.BatchCost` takes many resources in one request and returns
one result per resource, in request order. One resource's error does not fail
the batch. This plugin answers the ACTUAL query type only. Other query types
get a per-resource `Unimplemented` error.

### 3. GetActualCostRequest.resource

`finfocus-spec` v0.7.4 added `GetActualCostRequest.resource`, a full
`ResourceDescriptor`. The plugin reads it first and falls back to the legacy
`tags`, `arn`, and `resource_id` fields.

### Deferred to v0.2.0

- Per-request credentials (`x-finfocus-credential-*` metadata). v0.1.0 reads
  the API key from `CLOUDZERO_API_KEY` only
- Settings advertisement in `GetPluginInfo`

FOCUS 1.4 fields are not populated. The documented CloudZero response has no
matching data.

## Architecture

### Component Design

```text
┌─────────────────────────────────────────┐
│  FinFocus Core                          │
│  ├─ Router (selects CloudZero plugin)   │
│  ├─ Cache (respects expires_at)         │
│  └─ Engine (orchestrates queries)       │
└──────────────┬──────────────────────────┘
               │ gRPC GetActualCost() / BatchCost()
               ▼
┌─────────────────────────────────────────┐
│  CloudZero Plugin                       │
│  ├─ Mapper (resource → dimension IDs)   │
│  ├─ Client (REST API, budget, retry)    │
│  ├─ Cache (month query, plugin-side)    │
│  └─ Plugin (gRPC service)               │
└──────────────┬──────────────────────────┘
               │ HTTP REST v2
               ▼
        CloudZero API
```

### 1. Dimension Mapping (Plugin Responsibility)

**Rationale**: Core stays provider-agnostic. The plugin owns the CloudZero
dimension logic.

**Input**: `GetActualCostRequest.resource` (a `ResourceDescriptor`), or the
legacy `tags`, `arn`, and `resource_id` fields when no descriptor is sent.

**Resource identity**: the plugin tries these identifiers in order and uses
the first one that yields a CloudZero resource name (CZRN):

1. The `pulumi:cloudId` tag, `arn`, `pulumi:arn` tag, `arn` tag, and `id` on
   the descriptor
2. The legacy `tags["pulumi:cloudId"]`, `arn`, and `resource_id`

An identifier that already starts with `czrn:` is used unchanged. An AWS ARN
is converted to `czrn:<provider>:<service>:<region>:<account>:<type>:<id>`.
CloudZero does not document the CZRN format. The shape matched a live account
on 2026-11-17 for EC2 instances, NAT gateways, Elastic IPs, volumes, and
network interfaces.

When no identifier yields a CZRN, an AWS resource whose bare id has a known
prefix (`i-`, `vol-`, `eni-`, `nat-`, `eipalloc-`) and whose region is known
is matched by the CZRN suffix `:<type>:<id>`. A NAT gateway needs this,
because Pulumi state has no ARN for it. The account is unknown, so that
resource's query filters on `Region` only. Azure and GCP ids are not
converted. Supply a `czrn:` value.

**Provider**: the query groups by `CloudProvider` and does not filter on it.
CloudZero's filter is case-sensitive (`AWS` matched 69 rows on the live
account, `aws` matched none) and its spellings are not documented. Each row
carries its `CloudProvider` value, and a row matches when that value equals
the resource's provider ignoring case. The provider is normalized first:
`-native` is dropped, `azurerm` becomes `azure`, and `google` becomes `gcp`.
No CloudZero spelling is hardcoded.

**Shared dimensions**: these filter the query. All of them are optional.

- `Account`: the AWS account in an ARN, the Azure subscription, or the GCP project
- `Region`: the region on the descriptor, else the region in the ARN
- Tag dimensions: only the pairs in `CLOUDZERO_TAG_DIMENSIONS`
  (`tagKey=dimensionID`). Dimension IDs are never invented from tag names

**Example**:

```text
Input ARN: arn:aws:ec2:us-east-1:123456789012:instance/i-0abc123
Tags: {"Team": "platform"}
CLOUDZERO_TAG_DIMENSIONS: Team=User:Defined:Team

Resource ID: czrn:aws:ec2:us-east-1:123456789012:instance:i-0abc123
Provider: aws (rows match CloudProvider "AWS" ignoring case)
Shared filters:
{
  "Account": "123456789012",
  "Region": "us-east-1",
  "User:Defined:Team": "platform"
}
```

A resource with no identifier that yields a CZRN is declined with
`InvalidArgument`.

### 2. Caching Strategy (Multi-Tier)

#### Layer 1: Plugin-Side Cache

- **Query**: one month-aligned daily query per set of shared filters, grouped
  by the resource dimension. The resource itself is not a filter, so every
  resource with the same shared filters reads the same cached rows
- **Storage**: In-memory map (thread-safe RWMutex)
- **TTL**: Computed by this plugin. CloudZero responses have no TTL field.
  The newest two days use `CLOUDZERO_RECENT_TTL`. Older days use `CLOUDZERO_CACHE_TTL`
- **Eviction**: Time-based expiration on read
- **Errors**: never cached

**Rationale**:

- The documented costs limit is 60 requests a day. Cursor pages do not count.
  The old figure of about 1000 requests a month is not in the CloudZero docs
- Core clamps each resource's start to its creation date. A cache keyed on the
  exact window would spend one call per resource
- Example: 10 resources in the same account and region, over the same month,
  cost one call

#### Layer 2: Core-Side Cache

- **Scope**: Persistent cache across CLI invocations
- **TTL**: Derived from `expires_at`, capped by core

**Decision Flow**:

```text
Core cache hit? → Return
  ↓ (miss or stale)
Plugin cache hit for the month and shared filters? → Slice locally, return
  ↓ (miss or stale)
Query CloudZero API (spends one request from the daily budget)
  ↓
Update plugin cache (TTL computed by the plugin)
Return rows with expires_at computed per day
```

### 3. Batch Processing Semantics

**Request**: `BatchCostRequest` with an array of `ResourceDescriptor`

**Processing**:

1. **Readiness**: a missing API key or resource dimension fails every
   resource with `FailedPrecondition`
2. **Mapping**: each resource is mapped as in section 1. A resource that cannot be
   mapped gets its own `InvalidArgument` error
3. **Grouping**: resources with the same shared filters form one group. Each
   group is one query, or a cache hit
4. **Result Assembly**: one result per input resource, in request order. One
   group's API error does not fail the other groups

**Example**:

```text
Input: [EC2 i-1 in 111/us-east-1, EC2 i-2 in 111/us-east-1, Lambda in 111/us-west-2]
Shared filter sets: [{Account: 111, Region: us-east-1}, {Account: 111, Region: us-west-2}]
API Calls: 2
Output: [result for i-1, result for i-2, result for Lambda]
```

## API contract

Facts below come from CloudZero's public docs (captured 2026-10-04 in
`finfocus-pm` `research/cloudzero-api-facts.md`). They are not yet confirmed
on a live account. See the README "v0.1.0 limitations" section.

### Authentication

- **Status**: Documented. Not confirmed live
- **Method**: bare API key in the `Authorization` header. No `Bearer` prefix
- **API key**: `CLOUDZERO_API_KEY`
- **Scopes**: `billing:read_costs` and `billing:read_dimensions`

### Rate limiting

- **Status**: "60 requests/day" is documented on the costs page. The scope
  (key, organization, calendar day) is not documented
- **429**: documented. Do not retry in a loop. The plugin holds budgeted
  requests for the wait the body states, and for at least 5 minutes
- **`Retry-After` and `X-RateLimit-*`**: not found in the docs
- **503**: documented for telemetry writes, not for the costs read

### Billing endpoint

- **Status**: Schema documented. Not confirmed live
- **Endpoint**: `GET /v2/billing/costs`. There is no `GET /v2/billing`
- **Parameters**: `start_date`, `end_date`, `granularity`, repeated `group_by`,
  URL-encoded JSON `filters`, `cost_type`, `align_to_granularity`, `cursor`.
  The plugin sends `granularity=daily` and `align_to_granularity=false`, since
  it already aligns the window to month boundaries
- **Row**: `usage_date` (RFC3339), `cost` (JSON number), plus one string key
  per `group_by` dimension ID
- **Currency**: not in the response. `CLOUDZERO_CURRENCY` is an assumption
- **Pagination**: `pagination.cursor.next_cursor` and `has_next`. 10,000 rows
  per page. A cursor expires after 24 hours (HTTP 410). Restart once

### Budgets endpoint

- **Status**: Schema documented. A live call on 2026-11-19 returned HTTP 200
  with an empty list, so the read key works, but no budget has been seen live
- **Endpoint**: `GET /v2/budgets?expand=current`, paged by the same cursor as
  costs. The first page spends one request from `CLOUDZERO_DAILY_BUDGET`
- **Budget**: `id`, `name`, `view.id`, `granularity`, `planned_limits.monthly`
  (a date key to `{"amount"}`), `alerts` (a percentage key to `"Y"` or `"N"`),
  and with `expand=current`, `current: {planned, actual}`. Amounts are parsed
  from numbers or numeric strings, and date keys from `YYYY-MM-DD` or `YYYY-MM`
- **Mapping**: monthly budgets only. The limit is the planned amount for the
  current UTC month, and the status carries the actual as current spend.
  Enabled alerts become actual-spend thresholds. Core derives percentage used,
  forecast, and health, so the plugin never forecasts

### Recommendations endpoint

- **Status**: checked live on 2026-11-19 with the existing read key: HTTP 200,
  eight "Upgradable EBS Volumes" (`CIR-AWS-00010`) rows worth $0.83 together.
  The first call returned HTTP 504 and a retry seconds later succeeded.
  Through core the same day, the repeated `status` values were accepted. All
  of the recommendations are on the root volumes of the instances in stack
  version 131, which were replaced on 2026-11-18. With a core that sends
  target attributes (rshade/finfocus#2281), the version 131 state got all six
  open recommendations, one per instance (#29)
- **Endpoint**: `GET /v2/optimize/recommendations` with
  `cost_impact_last_30_days[gte]=0.01` and repeated `status` values
  `not_started`, `in_progress`, and `ignored`. CloudZero's default floor of
  1.00 returned nothing on that account. Paged by the same cursor as costs.
  The first page spends one request from `CLOUDZERO_DAILY_BUDGET`
- **Row**: `recommendation_id`, `title`, `recommendation_type_id` and `_name`,
  `effort`, `status`, `resource` (a CZRN), `resource_type`, `resource_name`,
  `cost_impact_last_30_days`, `created` (Unix seconds), and the lists
  `account`, `region`, and `cloud_provider`. Rows without an id or resource
  are dropped
- **Mapping**: one organization-wide fetch, matched to core's
  `target_resources` with `Mapper.Identity` and `Filter.Matches`, so a
  resource needs no resource dimension. An instance also matches its root
  volume, named by `attributes.rootBlockDevice.volumeId`
  (`Filter.MatchesRootVolume`). A match carries the target's id, and
  `cloudzero_resource` keeps the CZRN.
  Category cost, action type other, priority unspecified. Savings are the
  cost impact scaled from 30 days to the requested period. `ignored` ids go to
  the SDK's dismissed list, then the SDK applies the filter, paging, and
  summary

### Verified on a live account (2026-11-17)

- Bare-key auth and the costs response shape
- The `Resource` dimension returns one row per resource, valued as a CZRN
- `CloudProvider` is `AWS` for AWS resources, and the filter is case-sensitive:
  `aws` matches no rows
- A bad key returns HTTP 403 with `{"message": ...}`
- The 60 request limit is a rolling 24 hours, shared with other callers of the
  key or organization. The 429 body says how many minutes to wait
- Root EBS volumes and the IPv4 charge of an Elastic IP are separate CZRNs
  (`volume` and `network-interface`). An instance whose attributes name its
  root volume (`rootBlockDevice.volumeId`) gets the volume's rows as a separate
  `... root volume` source (#10). The EIP's IPv4 charge is not attributed
- `end_date` is exclusive (2026-11-18): a query ending at `2026-11-12T00:00:00Z`
  returns days up to 2026-11-11
- Data is near real time (2026-11-18): the current UTC day appears as a partial
  row, and the previous day is present. When a day becomes final was not measured

### Still open

- Azure and GCP `CloudProvider` spellings. The owner's organization has AWS
  data only
- Currency of `cost` for a non-USD account

### Date range and dimensions

`start_date` and `end_date` are ISO 8601 datetimes. This plugin sends UTC.
`end_date` is exclusive, so a month query from the first of the month to the
first of the next covers exactly that month. The plugin keeps the days that
overlap the request.

Dimension IDs in the docs include `Account`, `Service`, `CloudProvider`, and
`Region`. Tag and custom dimensions are per account. Configure them. Do not
hard-code `team` or `cost_center`. Service names such as `ec2` are not
documented as dimension values.

## Error Handling Strategy

**Errors returned by plugin**:

| Scenario | ErrorCode | Message | Fallback Hint |
| ---------- | ----------- | --------- | --------------- |
| API key missing | FailedPrecondition | "set CLOUDZERO_API_KEY" | n/a |
| Resource dimension unset | FailedPrecondition | "set CLOUDZERO_RESOURCE_DIMENSION" | n/a |
| Missing cloud id or bad window | InvalidArgument | fixed text with no key, URL, or body | n/a |
| Bad `page_token` or `projection_period` | InvalidArgument | "invalid page_token", or the accepted periods | n/a |
| HTTP 401 or 403 | PermissionDenied | "CloudZero rejected the API key" | n/a |
| HTTP 429 | ResourceExhausted | "60 requests in a rolling 24 hours; retry in N minutes", or "retry later" when the body states no wait | n/a |
| Budget spent | ResourceExhausted | "budget is exhausted; retry tomorrow" | n/a |
| HTTP 5xx after retries | Unavailable | "CloudZero is unavailable" | n/a |
| Malformed body | Unavailable | "CloudZero response is not valid JSON" | n/a |
| HTTP 410 twice | Unavailable | "CloudZero page cursor expired" | n/a |
| Network error or timeout | Unavailable | "CloudZero is unavailable" | n/a |
| Empty `costs` array | success, no rows | not a `$0` row | RECOMMENDED |

**Core behavior**:

- `FALLBACK_HINT_REQUIRED`: Core tries fallback plugins
- `FALLBACK_HINT_RECOMMENDED`: Core tries fallback if available, otherwise returns error
- `FALLBACK_HINT_NONE`: Core accepts error (CloudZero is definitive source)

## Future Enhancements

### v0.1.0 (Current)

- Actual cost and batch actual cost against the documented costs API
- Cursor pagination, a short retry, and a per-process request budget
- Plugin-side month cache

### After v0.1.1

- Budgets from `GET /v2/budgets` (`add-cloudzero-budgets`)
- Recommendations from `GET /v2/optimize/recommendations`
  (`add-cloudzero-recommendations`)

### v0.2.0

- Per-request credentials
- Settings advertisement
- Fixes from the live capture in `plan.md` phase K

### Later

- Parallel batch queries
- Metrics (API calls, cache hit rate, latency)
- Cost breakdown by component (compute, storage, data transfer)

## References

- **finfocus-spec v0.7.12**: Protocol definition at `proto/finfocus/v1/costsource.proto`
- **finfocus-spec CLAUDE.md**: SDK patterns and best practices
- **finfocus core**: Cache implementation at `internal/engine/cache/ttl.go`
- **CloudZero Docs**: Public API documentation (TBD: insert URL once verified)
