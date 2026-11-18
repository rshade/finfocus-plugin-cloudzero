# CloudZero plugin: plan to v0.1.0

Written 2026-11-15. Every measurement below was taken on 2026-11-15 in this directory. Nothing was committed, and
no CloudZero API call was made. Paths are relative to this repo unless they name another repo (`rshade/<repo>`).

Sources: `TASKS.md` and `docs/design.md` here; `finfocus-pm` `plugins/portfolio.md` (Family standard, Order to
v0.1.0, Release and dependency settings), `finfocus-pm` `research/cloudzero-api-facts.md` (API facts, cited
below as "facts §N"), the released siblings `rshade/finfocus-plugin-flexera` and `rshade/finfocus-plugin-vantage`
(both v0.1.1), and core `rshade/finfocus` (`CLAUDE.md` Registry section, `internal/registry/registry.json`,
`internal/conformance/`).

## Verdict

The release plumbing is mostly in place, and the cost path is not built. `GetActualCost` returns an empty list,
the API key is never read, and the HTTP client calls an endpoint that CloudZero does not document, with the wrong
auth header. The plugin also advertises projected cost, pricing spec, and estimate, then answers them with `$0`,
which core treats as a real price.

A v0.1.0 that follows the flexera precedent (built and tested against fakes only, labelled unverified) needs no
CloudZero key. The 2026-10-04 API research moved most of the `TASKS.md` items marked ⚠ (blocked on the key) to
"buildable from documented schema". Twenty agent tasks (2 L, 6 M, 12 S) and ten owner actions remain, plus two
agent and two owner steps once a key exists. The key is still needed before the plugin can honestly claim
resource-level costs, and before a live test, so it blocks a useful registry entry more than it blocks the tag.

## 1. Current state (measured 2026-11-15)

### Build, test, lint, release check

Local toolchain: `go version go1.27.4 linux/amd64`. `go.mod` says `go 1.27.1`.

| Command | Result |
| --- | --- |
| `go build ./...` | Exit 0 |
| `make test` (`go test -v ./...`) | 5 packages `ok`, 57 test and subtest runs, 0 failures. `cmd/` has no tests |
| `go test -race ./...` | All `ok` |
| `go test -cover ./...` | cache 100.0%, czclient 93.3%, mapping 100.0%, plugin 73.9%, cmd 0.0% |
| `go vet ./...` | Clean |
| `golangci-lint run ./...` (2.14.0, the `mise.toml` pin) | `0 issues.` (a deprecation warning for `exhaustruct` only) |
| `goreleaser check` (2.18.2) | Fails: `scm releases: current folder is not a git repository`. Expected before `git init` |
| `govulncheck ./...` | 5 reachable vulnerabilities, all in `golang.org/x/net` v0.59.0, fixed in v0.60.0 (GO-2026-6603, -6610, -6611, -6612, -6617); 2 more in required modules that the code does not call |
| `finfocus plugin conformance` (core `v0.5.6-6-g835e5f94`, scratch `FINFOCUS_HOME`, no key) | 10 total, 6 passed, 4 failed (see below) |

Core conformance output, summarized:

```text
✓ Name_ReturnsPluginIdentifier        ✓ Name_ReturnsProtocolVersion
✓ GetProjectedCost_ValidResource      ✗ GetProjectedCost_InvalidResource   (expected error, got success)
✗ GetProjectedCost_PermissionDenied   ✗ GetProjectedCost_InternalError     ✗ GetProjectedCost_Unavailable
✓ Context_Cancellation                ✓ Timeout_Respected                  ✓ Batch_Handling
Total: 10 | Passed: 6 | Failed: 4
```

The six passes come from the `$0` projected stub, not from working code. See risk R6.

The GoReleaser snapshot proof (`GORELEASER_CURRENT_TAG=v0.1.0 goreleaser release --snapshot --clean
--skip=publish`) was recorded as passing in a scratch copy on 2026-10-06 (`TASKS.md`, release section). It was
not rerun, because it needs a git repository.

### Spec pin

`go.mod` pins `github.com/rshade/finfocus-spec v0.7.0`. Core pins v0.7.12. The family standard in
`portfolio.md` says v0.7.5, and flexera and vantage are on v0.7.5. Since v0.7.0 the spec added things this plugin
needs:

- v0.7.4: `GetActualCostRequest.resource`, a `ResourceDescriptor` carrying provider, type, region, and inputs.
- v0.7.9: SDK conformance honors declared capabilities.
- v0.7.12: `SupportsRequest` names the capability the host is asking about.

### RPCs

| RPC | State | Evidence |
| --- | --- | --- |
| `Name` | Implemented | `internal/plugin/plugin.go` |
| `GetPluginInfo` | SDK-provided from `pluginsdk.ServeConfig.PluginInfo` (providers aws, azure, gcp; no explicit capabilities) | `cmd/finfocus-plugin-cloudzero/main.go` |
| Capabilities advertised | Inferred by the SDK: PROJECTED_COSTS, ACTUAL_COSTS, PRICING_SPEC, ESTIMATE_COST, BATCH_COST | spec v0.7.0 `pluginsdk/plugin_info.go` `inferCapabilities`, plus `BatchCost` matching `BatchCostHandler` |
| `Supports` | Stub: `Supported: true` for any non-nil resource | `plugin.go` |
| `GetActualCost` | Stub: empty `Results` with `FALLBACK_HINT_RECOMMENDED`; never calls the client, mapper, or cache | `plugin.go` |
| `GetProjectedCost` | Stub: `$0` with "not yet configured" | `plugin.go` |
| `GetPricingSpec` | Stub: empty spec with `Provider: "cloudzero"` | `plugin.go` |
| `EstimateCost` | Stub: `$0` | `plugin.go` |
| `BatchCost` | Stub: every resource gets a `FailedPrecondition` error | `plugin.go` |

The supporting packages exist but are not wired to any RPC:

- `internal/czclient`: `GET {base}/billing`, returns `struct{}{}`. It has no response type, no pagination, and no
  retry.
- `internal/mapping`: emits lowercase keys (`provider`, `resource_type`, `region`, `account`) and strips a
  `cloudzero:` tag prefix.
- `internal/cache`: an in-memory TTL map.

### Authentication

There is none in practice. `main.go` never reads an environment variable. `czclient.SetAPIKey` is called only
from tests, so a running plugin always has an empty key. When a key is set, the client sends
`Authorization: Bearer <key>`. CloudZero documents the bare key with no `Bearer` prefix (facts §1, H).
`CLOUDZERO_API_KEY` appears only in `README.md`.

### Repository state

The directory is not a git repository, there is no GitHub repo, the plugin is not in core's
`internal/registry/registry.json`, and it has no releases. `TASKS.md` has 0 of 9 Success Criteria boxes checked.

## 2. Gaps against the family standard

"Relation to `TASKS.md`" says whether `TASKS.md` already tracks the gap. The reconciliation table after this one
maps every `TASKS.md` task to this plan.

| # | Gap | Evidence | Relation to `TASKS.md` | Plan task |
| --- | --- | --- | --- | --- |
| G1 | Not a git repo, no GitHub repo, no `RELEASE_PLEASE_TOKEN` secret | Directory listing; `TASKS.md` release section | Stated in the release section, no task | O-3, O-4, O-5 |
| G2 | `.gitignore` line `finfocus-plugin-cloudzero*` matches the `cmd/finfocus-plugin-cloudzero/` directory, so a first `git add .` would leave out `main.go` | `.gitignore`; `git check-ignore -v --no-index` with that pattern reports `cmd/finfocus-plugin-cloudzero/main.go` | Not tracked | CZ-01 |
| G3 | Hand-written `[0.1.0] - 2026-09-30` entry in `CHANGELOG.md`; no `.markdownlintignore` | `CHANGELOG.md`; family rule 6 (release-please owns the changelog, and its output fails MD012 and MD004) | Not tracked | CZ-02 |
| G4 | `renovate.json` is not the family file: `config:base`, no `gomodTidy`, and it ignores `golang.org/x/net`, which is why G9's vulnerable x/net stayed | Diff against flexera's `renovate.json` | Task 7.0 marks it done | CZ-03 |
| G5 | `commitlint.config.js` adds a `type-enum` that rejects `build`; the family uses plain `config-conventional` | Diff against flexera | Task 7.0 marks it done | CZ-03 |
| G6 | `commitlint.yml`, `prose.yml`, `test.yml` use `actions/checkout@v4` and `jdx/mise-action@v2`; flexera uses v7 and v5 and has an actionlint job. `release.yml` and `release-please.yml` match flexera's (comments differ only) | `.github/workflows/` | Task 7.0 marks it done | CZ-04 |
| G7 | Release guard tests cover asset names only. Missing: release-please keys (`include-component-in-tag`, `initial-version`, `bump-patch-for-minor-pre-major`), manifest semver, no docker, brews, or nfpms sections, no `docker/build-push-action`, the `formats` key (the test reads the old `format` key) | `internal/goreleaser/goreleaser_test.go` has 2 tests; flexera's `goreleaser_test.go` has `TestReleasePleaseTagOmitsComponent` and `TestReleasePleaseBumpsPatchBeforeOneZero` | Release section asks for it, no task | CZ-05 |
| G8 | Release token, scopes, expiry, and rotation not documented | `grep RELEASE_PLEASE_TOKEN` finds nothing in `CLAUDE.md`, `README.md`, or docs | Release section asks for it, no task | CZ-06 |
| G9 | Spec v0.7.0 (core v0.7.12); `golang.org/x/net` v0.59.0 with 5 reachable vulnerabilities | `go.mod`; `govulncheck` | Not tracked (the "Known Vulnerabilities" table lists only GO-2026-6443) | CZ-07 |
| G10 | Advertised capabilities are false: projected, pricing spec, and estimate return `$0` or empty answers, and core's router treats `$0.00` as a valid result that does not fall back | `plugin.go`; SDK `inferCapabilities`; core `CLAUDE.md` Router section | Not tracked; `README.md` presents the `$0` placeholder as intended | CZ-08 |
| G11 | `Supports` says yes to every resource and capability | `plugin.go` | Not tracked | CZ-08 |
| G12 | API key never loaded; no config surface | `main.go`; `czclient.go` | Task 1.1 | CZ-09 |
| G13 | Client: wrong path (`/v2/billing`), `Bearer` prefix, untyped response, no cursor pagination, no 410, 429, or 403 handling, no request budget | `internal/czclient/client.go`; facts §1-§5, §11 rows 1-5 and 14 | Tasks 1.2, 2.1-2.3, written against the old, wrong API model | CZ-10 |
| G14 | Mapping emits keys that are not CloudZero dimension IDs and invents IDs from tag names | `internal/mapping/mapping.go`; facts §4, §11 rows 7-8 | Tasks 3.1-3.2 | CZ-11 |
| G15 | `GetActualCost` and `BatchCost` return nothing | `plugin.go` | Tasks 4.1-4.3 | CZ-12, CZ-13 |
| G16 | No in-repo conformance run (opencost runs the SDK suite in `test/conformance`) and no live test behind a build tag (flexera: `test/integration/live_test.go`) | Missing | Task 6.2 (live only) | CZ-15, CZ-16 |
| G17 | No `plugin.manifest.json` (flexera has one, vantage does not, so the family does not require it) | Missing | Not tracked | CZ-17 |
| G18 | Docs state wrong API facts | `README.md` (Bearer, `/v2/billing`, `page`/`page_size`, `X-RateLimit-*`, "TTL hints from CloudZero API responses"); `CLAUDE.md` lines 72, 192, 194, 198; `docs/design.md` lines 170 ("~1000 requests/month"), 239, 255, 343 | Task 6.3, without the corrections | CZ-18 |
| G19 | No "v0.1.0 limitations" section saying the plugin is unverified against the live API (flexera's README has one) | `README.md` | Not tracked | CZ-18 |
| G20 | Release not proven: no snapshot build since 2026-10-06, no tag, no assets, no registry entry | `TASKS.md` release section; `registry.json` | Tasks 7.1, 7.2 | CZ-20, O-8, O-10 |

### Reconciling `TASKS.md`

`TASKS.md` stays as it is for now. CZ-19 rewrites it once the owner accepts this plan, so the two do not drift.

| `TASKS.md` task | Disposition | Why | Plan task |
| --- | --- | --- | --- |
| 1.1 Env vars | Keep; drop the `CLOUDZERO_API_SECRET` fallback | Only one key exists (facts §11 row 9) | CZ-09 |
| 1.2 Client config | Keep; bare key, treat 401 and 403 both as auth failure | facts §1, §5 | CZ-10 |
| 1.3 Per-request credentials | Defer to v0.2.0 | `TASKS.md` already lists it as nice-to-have | Q6 |
| 2.1 Billing query | Rewrite to `GET /v2/billing/costs`; no longer blocked, because the schema is documented (H) | facts §3 | CZ-10 |
| 2.2 Pagination | Rewrite around `cursor`, `has_next`, and 410 restart; drop `page_size` and `next_page_token` | facts §3, §11 row 4 | CZ-10 |
| 2.3 Retry and backoff | Keep, narrowed: retry transport errors and 5xx briefly; 429 becomes `ResourceExhausted` with no loop | 60 requests a day (facts §5) | CZ-10 |
| 3.1 ARN parsing | Keep; add Azure subscription and GCP project | Account dimension | CZ-11 |
| 3.2 Tag extraction | Rewrite as configured tag-to-dimension-ID mapping | facts §4, §9 trap 9 | CZ-11 |
| 3.3 Deduplication | Fold into batch and cache | The request budget forces it | CZ-12, CZ-13 |
| 4.1 GetActualCost | Keep; this is the core of v0.1.0 | none | CZ-12 |
| 4.2 Cache | Promote from nice-to-have to v0.1.0 | Without it the 60 a day budget runs out | CZ-12 |
| 4.3 BatchCost | Keep, actual query type only | none | CZ-13 |
| 4.4 FOCUS 1.4 fields | Drop from v0.1.0 | The documented response has no such fields (facts §11 row 13) | none |
| 5.1 Error mapping | Keep | none | CZ-14 |
| 5.2 Validation | Keep | none | CZ-14 |
| 6.1 Coverage | Keep | none | CZ-15 |
| 6.2 Integration tests | Split: fake-backed binary and conformance tests (agent), live test (needs key) | none | CZ-15, CZ-16, K-3 |
| 6.3 Docs | Keep, with the corrections in G18 | none | CZ-18 |
| 7.0 Toolchain | Mostly done; residual gaps G2-G8 | none | CZ-01 to CZ-06 |
| 7.1 Registry entry | Keep, after a release with assets | Core registry rules | O-10 |
| 7.2 Release workflow | Keep the release-please flow; drop "signed binaries" and `make build-all-platforms` | Not in the family standard; GoReleaser builds every platform | CZ-20, O-8 |

## 3. Ordered task list to v0.1.0

Sizes: S is one small change with its test, M is one focused session, L is more than one session. The sizes are
agent guesses, like every estimate in `TASKS.md`.

### Owner decisions first

| ID | Decision | Why it comes first |
| --- | --- | --- |
| O-1 | Release bar: ship v0.1.0 tested against fakes only and labelled unverified (flexera precedent), or wait for the key | Decides whether phase K gates the tag |
| O-2 | Cost contract when no resource-level dimension is configured: decline in `Supports` (recommended), or return the cost of a configured dimension slice, such as a tag mapped to a custom dimension, labelled as slice-level | Slice costs attributed to several resources double count in stack totals (R2) |

### Phase A: agent work, no git needed (repo hygiene)

| ID | Task | Size | Verify |
| --- | --- | --- | --- |
| CZ-01 | Anchor the binary ignore rule (`/finfocus-plugin-cloudzero`, `/bin/`) so `cmd/` is not ignored | S | After `git init`, `git check-ignore cmd/finfocus-plugin-cloudzero/main.go` prints nothing |
| CZ-02 | Reset `CHANGELOG.md` to a heading-only file for release-please; add `.markdownlintignore` with `CHANGELOG.md` | S | `markdownlint-cli2 "**/*.md"` passes |
| CZ-03 | Copy the family `renovate.json` (flexera or aws-public) and plain `config-conventional` commitlint config | S | Diff against flexera is empty |
| CZ-04 | Align `commitlint.yml`, `prose.yml`, `test.yml` with flexera (checkout v7, mise-action v5, actionlint job); keep `release.yml` and `release-please.yml` as they are | S | `actionlint .github/workflows/*.yml` exits 0 |
| CZ-05 | Release guard tests: port flexera's release-please tests; add `initial-version`, manifest semver at or above 0.0.0, no docker, brews, or nfpms sections, no `docker/build-push-action`, read `formats` | S | Reintroduce each defect in turn and its test fails |
| CZ-06 | Document `RELEASE_PLEASE_TOKEN` (fine-grained, this repo, Contents, Pull requests, and Issues read and write, Metadata read), its expiry, and how to rotate it, in `CLAUDE.md` or `CONTRIBUTING.md` | S | Present |

### Phase B: agent work against fakes (the cost path)

| ID | Task | Size | Depends on | Verify |
| --- | --- | --- | --- | --- |
| CZ-07 | Bump `finfocus-spec` to v0.7.12 (core's pin) and `golang.org/x/net` to v0.60.0 or later; `go mod tidy` | S | none | `govulncheck` shows no reachable x/net finding; build and tests pass |
| CZ-08 | Honest capabilities: set `PluginInfo.Capabilities` to ACTUAL_COSTS (add BATCH_COST in CZ-13); `GetProjectedCost`, `GetPricingSpec`, `EstimateCost` return `codes.Unimplemented`; `Supports` declines any capability other than actual cost, providers other than aws, azure, gcp, and requests when no key is set, each with a reason | M | CZ-07 | Unit tests per case; `GetPluginInfo` lists exactly the declared set |
| CZ-09 | Config: `CLOUDZERO_API_KEY` (required for cost RPCs, never logged), `CLOUDZERO_BASE_URL` (default `https://api.cloudzero.com/v2`, used by fakes), `CLOUDZERO_COST_TYPE` (default `real_cost`, checked against the 8 documented values), `CLOUDZERO_CURRENCY` (default `USD`, a recorded assumption), dimension mapping, daily request budget, cache TTL. The plugin starts without a key and the cost RPCs then return `FailedPrecondition` with a fix hint | M | CZ-07 | Table tests for every variable; a test asserts the key never appears in logs or errors |
| CZ-10 | Rewrite `czclient`: typed `GetCosts` and `GetDimensions`; `/billing/costs`; bare `Authorization`; `User-Agent`; repeated `group_by` through `url.Values.Add`; `filters` as JSON; cursor loop on `has_next` and `next_cursor`; one rerun on 410; typed errors for 400, 401 and 403, 429, 5xx; both error body shapes; 30 s timeout; a per-process counter that stops before the daily budget | L | CZ-09 | `httptest` fakes built from facts §3: two pages, 410, 429, 403, 400, malformed body. These prove the client matches the docs, not CloudZero |
| CZ-11 | Rewrite mapping to CloudZero dimension IDs (`CloudProvider`, `Account`, `Region`, plus configured `tag key → dimension ID` pairs and an optional resource dimension). Read `GetActualCostRequest.resource` first, then tags (`pulumi:cloudId`), then `arn`. No hard-coded `team` or `cost_center` | M | CZ-07, CZ-09 | Table tests for AWS ARNs, Azure IDs, GCP names, missing data |
| CZ-12 | `GetActualCost`: validate the window; build filters; query a month-aligned daily window that covers the request, so one call serves many resources, and slice it locally (core clamps each resource's start to its creation date, so exact-window cache keys would spend one call per resource); one `ActualCostResult` per day with `source` naming CloudZero and the cost type; `expires_at` computed by the plugin with a short TTL for the newest 2 days; an empty answer is "no data", never `$0`; wire `internal/cache` | L | CZ-10, CZ-11, O-2 | Fake-backed tests: cache hit, miss, partial month, empty result, 429, budget exhausted |
| CZ-13 | `BatchCost` for the actual query type: group resources by window and filter shape, one query per group, per-resource errors, order kept; a projected query type gets per-resource `Unimplemented`; declare BATCH_COST | M | CZ-12 | Tests: 1 resource, many, duplicates (one upstream call), partial failure, empty input |
| CZ-14 | Error and validation pass: map to the statuses core expects (`InvalidArgument` as a decline, `Unavailable`, `ResourceExhausted`, `FailedPrecondition`, `PermissionDenied`); never forward an upstream status as is; no key, URL query, or body in messages | S | CZ-10 to CZ-13 | One test per mapping |
| CZ-15 | Tests: the SDK conformance suite in `test/conformance` (opencost pattern) with an AWS sample resource; a binary smoke test (starts, announces its port, `GetPluginInfo` lists only declared capabilities); coverage of at least 80% overall and 95% on client and actual cost | M | CZ-14 | `go test ./...`, `go test -cover ./...` |
| CZ-16 | Live test scaffold: `//go:build integration`, skips without `CLOUDZERO_API_KEY`, at most 3 calls; `make test-integration` | S | CZ-12 | Compiles with `-tags=integration` and skips without a key. Only the owner runs it with a key |
| CZ-17 | `plugin.manifest.json` in the flat `registry.Manifest` shape (name `cloudzero`, type `costsource`, executable, configuration with env names) | S | CZ-09 | Core `plugin validate` against an install from a local build |
| CZ-18 | Docs: rewrite `README.md` (bare-key auth, the fixed scopes `billing:read_costs` and `billing:read_dimensions`, 60 requests a day, cost type, currency assumption, mapping config, a "v0.1.0 limitations" section saying it is built from CloudZero docs and not verified against the live API); correct `CLAUDE.md` and `docs/design.md` per facts §11 | M | CZ-12 | `make lint-md`, `make lint-prose` |
| CZ-19 | Rewrite `TASKS.md` to match this plan's dispositions, or replace it with a pointer to this plan | S | owner accepts this plan | `markdownlint-cli2` passes |
| CZ-20 | Full gate: `make lint`, `make test`, `make test-race`, `govulncheck`, `make lint-md`; after `git init`, `goreleaser check` and the snapshot build (archives plus `checksums.txt`); core `plugin inspect` and `plugin conformance` results recorded | S | All above, O-3 | Every command exits 0, except core conformance (R6) |

### Phase C: owner actions

| ID | Action | When |
| --- | --- | --- |
| O-3 | `git init`, review the tree (check CZ-01 first), first commit | After phase A; ideally after phase B |
| O-4 | Create `rshade/finfocus-plugin-cloudzero` and push. Whether it is public is the owner's decision; the core installer downloads release assets from GitHub, so a registry entry needs assets that users can download | After O-3 |
| O-5 | Add the `RELEASE_PLEASE_TOKEN` secret; run `Release Please` by `workflow_dispatch` and confirm it does not fail with `Input required and not supplied: token` or `Bad credentials` | Before the first release PR |
| O-6 | Install the Renovate app and confirm the Dependency Dashboard appears | After O-4 |
| O-7 | Obtain a CloudZero key with the fixed scopes `billing:read_costs` and `billing:read_dimensions` (and `billing:read_resource_tags` if wanted). `tasks.yaml` notes CloudZero pricing is a custom contract, so access may need sales contact | Any time; gates phase K |
| O-8 | Merge the release PR; check the tag is plain `v0.1.0`, the GoReleaser run is green, and `gh release view v0.1.0 --json assets --jq '.assets \| length'` is 7 (six archives plus `checksums.txt`) | After phase B, O-5, and O-1 |
| O-9 | Never hand-create a tag; if a release goes wrong, delete it and rerun `release.yml` by `workflow_dispatch` | Only if O-8 fails |
| O-10 | Core registry PR to `rshade/finfocus` (an agent may draft it on request): `registry.json` entry with `capabilities: ["cost_retrieval"]`, providers aws, azure, gcp, `min_spec_version: "0.7.12"`, `asset_hints` `asset_prefix: "finfocus-plugin-cloudzero"` and `version_prefix: false`; docs page, compatibility matrix, FAQ table, table of contents, `finfocus-install` skill references; validate with a fresh `FINFOCUS_HOME` (`plugin list --available`, `plugin install cloudzero` shows `Checksum verified (SHA256)`, `plugin list`, `plugin inspect`) | After O-8, and after K-1 if O-1 or Q3 says so |

### Phase K: needs the key

| ID | Task | Size | Who |
| --- | --- | --- | --- |
| K-1 | Run the capture plan in facts §10: about 8 calls, well inside 60 a day. It settles resource-grained dimensions, tag and custom dimension key forms, `CloudProvider` values for Azure and GCP, `end_date` semantics, bad-key status, rate-limit scope, `Retry-After`, currency, freshness. Redact account IDs | S | Owner |
| K-2 | Turn the sanitized captures into test fixtures; fix mapping defaults and error mapping to what was observed; pick the resource dimension if one exists | M | Agent |
| K-3 | Run `make test-integration` with the key | S | Owner |
| K-4 | Change the README and docs from "unverified" to "verified against the live API on DATE" for each settled item | S | Agent |

### What fakes can and cannot prove

Fakes are enough for everything in phases A and B. That covers the client against the documented schema,
pagination, 410 and 429 handling, the request budget, the cache, batching, error mapping, conformance, the
release, and the registry mechanics. Fakes cannot show:

- whether any CloudZero dimension returns one row per resource;
- which dimension IDs and value spellings an account has;
- whether a real key fails with 401 or 403;
- the currency of `cost`;
- how fresh the data is.

So a fakes-only v0.1.0 can be correct about the protocol and still return slices that do not match a Pulumi
resource. That is why O-2 defaults to declining.

## 4. Risks and open questions

### Risks

| # | Risk | Mitigation |
| --- | --- | --- |
| R1 | 60 requests a day on the costs endpoint (facts §5, H); whether that is per key or per organization is unverified. One overview of a large stack, or another tool on the same CloudZero organization, can use up the day | Month-aligned grouped queries, cache, batch, a budget below 60, `ResourceExhausted` with a "retry tomorrow" message (CZ-10, CZ-12, CZ-13) |
| R2 | Resource-level cost is unproven (facts §7, U). A slice cost attributed to every resource in the slice double counts in stack totals | Decline by default (O-2); K-1 decides |
| R3 | A fakes-only release encodes assumptions that may be wrong: Azure and GCP `CloudProvider` values, `end_date` inclusive or exclusive, 401 versus 403, missing currency (all U) | Label unverified in the README (CZ-18); phase K |
| R4 | `real_cost` drops tax, support, and unused commitments, so totals differ from aws-ce (`UnblendedCost`) and invoices | Make cost type configurable and name it in each result (CZ-09, CZ-12) |
| R5 | No currency in the response; USD is assumed | `CLOUDZERO_CURRENCY`, documented (CZ-09) |
| R6 | Core `plugin conformance` probes only `GetProjectedCost`. Once CZ-08 makes projected `Unimplemented`, `GetProjectedCost_ValidResource` and `Batch_Handling` will fail where they now pass against the `$0` stub. Core issue #2166 (probes use an AWS resource) does not bite here: this plugin lists aws, so the AWS probe resource is in scope. The problem is a different, related gap: core conformance does not skip probes for capabilities a plugin does not declare. `plugin certify` runs the same suite | Gate on the SDK suite (CZ-15), which honors declared capabilities since spec v0.7.9; record the core result in the registry PR; the owner decides whether to widen #2166 or file a capability issue (no agent writes to GitHub without approval) |
| R7 | With aws-ce and cloudzero both installed, both advertise actual cost for AWS resources. Which answer core shows, without routing config, is unverified here | Document routing priority in the README; check during O-10 |
| R8 | CloudZero access may need a commercial contract, so the key may never arrive | O-1 lets v0.1.0 ship without it; the registry entry waits on Q3 |
| R9 | The family standard has moved: `portfolio.md` says spec v0.7.5 and Go 1.27.1, core is on v0.7.12 and Go 1.27.4. Pinning v0.7.12 sets `min_spec_version` 0.7.12, which excludes core releases older than that spec | Q7 |
| R10 | `portfolio.md` dates its cloudzero recheck 2026-11-16, one day after this plan. The local measurements here agree with it on every count (10 Go files, 5 test files, 0 of 9, not a repo, not registered) | None needed; noted so the date is not read as later evidence |

### Open questions for the owner

1. Q1 (O-1): ship v0.1.0 against fakes only, as flexera did?
2. Q2 (O-2): decline, or slice costs, when no resource dimension is configured?
3. Q3: may the core registry entry go in before the K-1 capture run? Flexera's entry did go in on fakes only.
4. Q4: public repository? This is the owner's call, and the registry needs downloadable assets.
5. Q5: default `cost_type`: `real_cost` (CloudZero's default, closer to engineering spend) or `billed_cost`
   (matches invoices)?
6. Q6: per-request credentials (`TASKS.md` 1.3) and settings advertisement (spec v0.7.6, which lets core deliver
   the key from ESC and show it in the dashboard) in v0.1.0, or v0.2.0?
7. Q7: pin spec v0.7.12 (core's pin, needed for `Supports` capability and `GetActualCostRequest.resource`), or the
   portfolio's v0.7.5?
