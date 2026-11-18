# CLAUDE.md

This file provides guidance to Claude Code when working on the CloudZero plugin.

## Project Overview

FinFocus CloudZero Plugin is an actual-cost data source plugin that retrieves
historical billing data from the CloudZero REST API v2. It supports multi-cloud
environments (AWS, Azure, GCP) and maps Pulumi resources to CloudZero dimensions
for cost analysis.

## Quick Reference

```bash
make build          # Build the plugin binary
make test           # Run unit tests
make lint           # Run linter
make cover          # Coverage gate (80% overall, 95% client and plugin)
make validate       # Run fmt, vet, test, cover, lint, vuln
make dev            # Build and run with default port
```

## Protocol and RPC Methods

The plugin implements `CostSourceService` from `finfocus-spec`:

### Required Methods

- **Name()**: Returns "finfocus-plugin-cloudzero"
- **Supports(ResourceDescriptor)**: Checks if resource is supported
- **GetPluginInfo()**: Returns plugin metadata (name, version, spec_version, providers)

### Cost Query Methods

- **GetActualCost(GetActualCostRequest)**: Query historical costs for one resource
  - Prefer `resource` (a `ResourceDescriptor`), then legacy `tags`, `arn`, and `resource_id`
  - Output: one `ActualCostResult` per day that overlaps the window. Empty means no data
  - `source` names CloudZero, the cost type, and the configured currency
  - `expires_at` is computed by this plugin, not copied from CloudZero

- **GetProjectedCost(GetProjectedCostRequest)**: `codes.Unimplemented`. Do not return `$0`

- **BatchCost(BatchCostRequest)**: Multi-resource cost query
  - Input: Array of `ResourceDescriptor`
  - Output: one `ResourceCostResult` per resource, in request order. One
    resource's error does not fail the batch
  - Answers the ACTUAL `query_type` only. Every other query type gets a
    per-resource `Unimplemented` error

### Optional Methods

- **GetPricingSpec()**: `codes.Unimplemented`
- **EstimateCost()**: `codes.Unimplemented`
- **DryRun()**: not implemented. The plugin has no `DryRunHandler`, so the SDK server returns `Unimplemented`

## Architecture

### Request Flow

1. Core sends gRPC request with `ResourceDescriptor` (provider, resource_type, sku, region, tags, arn)
2. Plugin's `Mapper` normalizes to CloudZero dimensions:

   - Require `CLOUDZERO_RESOURCE_DIMENSION`. Decline when it is unset
   - Filters use dimension IDs: `CloudProvider`, `Account`, `Region`, plus configured tag mappings
   - Resource dimension value is a CZRN. An existing `czrn:` id wins. Otherwise an AWS ARN is converted to `czrn:<provider>:<service>:<region>:<account>:<type>:<id>`
3. Plugin's `CZClient` queries CloudZero REST API v2
4. Response cached via `Cache`. This plugin computes the TTL. CloudZero does not send one
5. Return `ActualCostResult` or error with proper `ErrorCode`

### Key Components

#### `czclient/client.go`

- HTTP client wrapper for CloudZero REST API v2
- Authentication is the bare API key in `Authorization`. There is no `Bearer` prefix
- `GET /v2/billing/costs` and `GET /v2/billing/dimensions`
- Cursor pagination (`cursor`, `has_next`, `next_cursor`). One restart on HTTP 410
- Retry transport errors and 5xx briefly. HTTP 429 is `ResourceExhausted` with no loop
- A per-process counter stops before `CLOUDZERO_DAILY_BUDGET` (default 50). Every
  attempt at a first page spends budget, retries included. Cursor pages do not

#### `mapping/mapping.go`

- Maps a resource to CloudZero dimension IDs
- AWS account from the ARN, Azure subscription, GCP project
- Tag filters come only from `CLOUDZERO_TAG_DIMENSIONS` (`tagKey=dimensionID`)
- Does not invent dimension IDs such as `team` or `cost_center` from tag names

#### `cache/cache.go`

- In-memory map with a per-entry expiry. Expired entries are not served, but
  nothing evicts them. There is no LRU or size limit
- The plugin sets each entry's TTL. CloudZero sends none
- Thread-safe (uses RWMutex)
- Holds one month query per set of shared filters, so many resources share it

#### `plugin/plugin.go`

- Implements `CostSourceService` gRPC interface
- Orchestrates the `CostSource` client, `Mapper`, and `Cache`. The cost path is
  in `actual.go` and `batch.go`
- `status.go` maps each `czclient.Kind` to a gRPC status code with a fixed
  message. The upstream body is never forwarded
- Logging via zerolog. There are no metrics

## Code Style

### General Rules

- Use testify `require` and `assert` only (no manual `if err != nil { t.Errorf(...) }`)
- All tests use `t.Parallel()` where safe (no shared state)
- Format with `gofmt -s`
- Lint with `golangci-lint run ./...`
- All exported functions have godoc comments

### Error Handling

```go
// ✓ Good: Use pluginsdk error codes
return nil, status.Error(codes.InvalidArgument, "message")

// ✓ Good: Wrap errors with context
return nil, fmt.Errorf("operation failed: %w", err)

// ✗ Wrong: Don't panic in libraries
panic("something went wrong")

// ✗ Wrong: Don't use custom error codes (use proto ErrorCode enum)
return &ErrorDetail{Code: 999, Message: "custom error"}
```

### Logging

```go
// ✓ Good: Use zerolog structured logging
p.logger.Warn().
    Str("arn", arn).
    Err(err).
    Msg("invalid arn format")

// ✗ Wrong: Don't log to stdout (except PORT announcement)
fmt.Println("debug info")

// ✗ Wrong: Don't use unstructured strings
p.logger.Info().Msg("got response: " + strconv.Itoa(code))
```

### Resource Naming

- Use consistent names for Pulumi resource types:
  - aws:ec2/instance:Instance (Pulumi format)
  - ec2 (short form, internal)
- Normalize formats early in request path
- Document accepted formats in function godoc

## Testing

### Unit Tests

- Place in same package with `_test.go` suffix
- Use table-driven tests for multiple cases
- Fake CloudZero in plugin tests through the `plugin.CostSource` interface
  (`fakeCosts` in `internal/plugin/plugin_test.go`). `*czclient.Client` implements it
- Test `czclient` itself against `httptest` servers

### Integration Tests

- `test/integration/live_test.go` is behind `-tags=integration`. It calls the
  live API through `czclient`, skips without `CLOUDZERO_API_KEY`, and makes at
  most three calls. Only the owner runs it
- `test/conformance` runs the SDK conformance suite against an `httptest` fake
- `cmd/finfocus-plugin-cloudzero/main_test.go` builds the binary, reads its
  `PORT=` line, and checks `GetPluginInfo` over gRPC

### Test Structure

```go
// ✓ Good: Parallel, isolated, named cases
func TestFilterCloudIdentifiers(t *testing.T) {
    t.Parallel()
    mapper := NewMapper(zerolog.Nop(), nil, "Resource")
    tests := []struct {
        name     string
        resource *pbc.ResourceDescriptor
        want     Filter
    }{
        {"aws arn", ...},
        {"nat gateway", ...},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
            got, err := mapper.Filter(tt.resource, nil, "", "")
            require.NoError(t, err)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

## CloudZero API Contract

### Verified (from public docs)

- **Base URL**: `https://api.cloudzero.com/v2`
- **Authentication**: bare API key in `Authorization` (no `Bearer` prefix)
- **Costs**: `GET /v2/billing/costs` with `start_date`, `end_date`, repeated `group_by`, JSON `filters`, `cost_type`, `cursor`
- **Dimensions**: `GET /v2/billing/dimensions`
- **Rate limit**: 60 requests a day on costs. Cursor pages do not count. Scope of the limit is unverified
- **Errors**: 400, 403, 410, and 429. 401 is not documented. Treat it as an auth failure.
  A 403 on dimensions uses `{"message":"..."}`. Other errors use `{"error":{"message","type"}}`

### Unverified (requires a sandbox key)

- Whether any dimension is one row per resource, and the value spelling
- Azure and GCP `CloudProvider` values
- Whether `end_date` is inclusive
- Whether a bad key is 401 or 403 on the wire
- Currency of `cost`, and how fresh the newest day is
- Whether the 60 request limit is per key or per organization

**Action**: The owner runs the capture plan in `plan.md` phase K. Do not call the live API from an agent session.

## Dependencies

### Compile-Time

- `github.com/rshade/finfocus-spec v0.7.12`: Proto definitions and SDK
- `google.golang.org/grpc`: gRPC client/server
- `google.golang.org/protobuf`: Protobuf runtime
- `github.com/rs/zerolog`: Structured JSON logging

### Test-Time

- `github.com/stretchr/testify`: assert/require for tests

### Build-Time

- golangci-lint: Linting
- gofmt: Code formatting

## Development Workflow

1. **Read the task** in TASKS.md
2. **Write tests first** (TDD) - place in same package with `_test.go` suffix
3. **Implement feature** to pass tests
4. **Run validation**: `make validate` (fmt, vet, test, cover, lint, vuln)
5. **Commit** (only when tests pass and lint is clean)

## Common Pitfalls

### ARN Parsing

- CloudZero dimension names may differ from resource URN formats
- Always normalize before querying (CloudZero may use lowercase, full names, etc.)
- Test with real AWS, Azure, GCP ARNs

### Pagination

- CloudZero pages with `cursor` and `has_next`, not `page_token`
- `next_cursor` is opaque. Pass it back exactly
- Empty `has_next` or a null cursor ends the loop. HTTP 410 means rerun the query once

### Rate Limiting

- The documented costs limit is 60 requests a day. Do not loop on 429
- `Retry-After` and `X-RateLimit-*` are not in the docs. Do not depend on them
- Return `codes.ResourceExhausted` with a retry-tomorrow message

### Cache Invalidation

- CloudZero responses have no TTL. This plugin sets `expires_at`
- Newest two days use a short TTL. Older days use `CLOUDZERO_CACHE_TTL`
- Do not cache an auth failure

## Release token

`RELEASE_PLEASE_TOKEN` is a fine-grained GitHub personal access token for this
repository only. Scopes: Contents read and write, Pull requests read and write,
Issues read and write, Metadata read. Set it as an Actions secret before the
first Release Please run.

GitHub shows the token once and sets its expiry at creation. Pick an expiry of
one year or less, and record the date in the team password store, not in git.
To rotate: create a new fine-grained token with the same scopes, update the
Actions secret, confirm `Release Please` succeeds from `workflow_dispatch`,
then delete the old token. A deleted key or token stops working immediately.

Never hand-create a release tag. release-please opens the release PR, and the
tag must be plain `v0.1.0` (`include-component-in-tag` is false).

## Session Learnings

- Do not advertise projected cost, pricing spec, or estimate. Core treats a
  `$0` projected result as a real price and will not fall back.
- `Supports` for an unspecified capability must be false. A true answer means
  the plugin covers every capability.
- The cost query is month-aligned and sliced locally. One call per resource
  exhausts the 60 request daily budget, so the query filters only on shared
  dimensions (`CloudProvider`, `Account`, `Region`, tag dimensions), never on
  the resource. `GetActualCost` and `BatchCost` share one cache entry per month
  and filter set.
- Resource identity is resolved once in `mapping.Filter`, in this order:
  descriptor `pulumi:cloudId` tag, `arn`, `id`, then the legacy request `tags`,
  `arn`, `resource_id`. The first one that yields a CZRN wins.
- `make cover` (`scripts/check-coverage.sh`) gates coverage at the plan.md
  CZ-15 targets: 80% overall, 95% on `internal/czclient` and `internal/plugin`.
- `align_to_granularity` is a documented costs parameter (facts §3). The
  research facts in `finfocus-pm` are more complete than `docs/design.md`.
- `finfocus-spec` v0.7.12 matches core. v0.7.4 added `GetActualCostRequest.resource`.
  v0.7.12 added the capability field on `SupportsRequest`.

## Recent Changes

- v0.1.0: actual cost and batch actual cost against the documented CloudZero
  costs API, tested with fakes. Projected cost, pricing spec, and estimate
  return Unimplemented. Not yet verified against a live account.
