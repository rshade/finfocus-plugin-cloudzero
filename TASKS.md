# Tasks

`plan.md` is the source of truth for the v0.1.0 release. This file records
the disposition of the original task list. The owner accepted that plan on
2026-11-15 with these defaults:

- Ship v0.1.0 tested against fakes and labelled unverified.
- Decline a resource when no resource dimension is configured.
- Default cost type is `real_cost`.
- Per-request credentials and settings advertisement wait for v0.2.0.
- Pin `finfocus-spec` v0.7.12.

| Original task | Disposition |
| --- | --- |
| 1.1 Environment variables | Done in `internal/config`. No `CLOUDZERO_API_SECRET` |
| 1.2 Client setup | Done. Bare API key. 401 and 403 are both auth failures |
| 1.3 Per-request credentials | Deferred to v0.2.0 |
| 2.1 Billing query | Done as `GET /v2/billing/costs` |
| 2.2 Pagination | Done with `cursor`, `has_next`, and one restart on 410 |
| 2.3 Retry | Done. Short retry for transport and 5xx. 429 is not retried |
| 3.1 ARN parsing | Done. Covers the Azure subscription id and the GCP project id |
| 3.2 Tag extraction | Done as configured `tag key=dimension ID` pairs |
| 3.3 Duplicate calls | Done in batch grouping and the month cache |
| 4.1 GetActualCost | Done |
| 4.2 Cache | Done. Required for the 60 request daily budget |
| 4.3 BatchCost | Done for the actual query type |
| 4.4 FOCUS 1.4 fields | Dropped. The documented response has none |
| 5.1 Error mapping | Done |
| 5.2 Validation | Done |
| 6.1 Coverage | Enforced by `make cover` in CI: 80% overall, 95% client and plugin |
| 6.2 Integration tests | Fake conformance is in `test/conformance`. Live test is `make test-integration` and needs a key |
| 6.3 Docs | README, CLAUDE.md, and docs/design.md match the 2026-10-04 API notes |
| 7.0 Release tooling | Release guards, Renovate, and workflows follow the family plugins |
| 7.1 Registry entry | Owner action after a GitHub release with downloadable assets |
| 7.2 Release workflow | release-please. No hand-cut tag |

Phase K in `plan.md` (capture, fixtures, live run, drop the unverified label)
stays blocked on a CloudZero key with scopes `billing:read_costs` and
`billing:read_dimensions`.
