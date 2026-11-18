# Tasks

`plan.md` is the source of truth for the v0.1.0 release. This file records
the disposition of the original task list. The owner accepted that plan on
2026-11-15 with these defaults:

- Ship v0.1.0 tested against fakes and labelled unverified.
- Decline a resource when no resource dimension is configured.
- Default cost type is `real_cost`.
- Per-request credentials and settings advertisement wait for v0.2.0.
- Pin `finfocus-spec` v0.7.12.

v0.1.0 was released on 2026-11-18 after a live check against the
`initial-30` `dev` stack. New behaviour changes go through OpenSpec (see
CLAUDE.md).

| Original task | Disposition |
| --- | --- |
| 1.1 Environment variables | Done in `internal/config`. No `CLOUDZERO_API_SECRET` |
| 1.2 Client setup | Done. Bare API key. 401 and 403 are both auth failures. A bad key returns 403 live |
| 1.3 Per-request credentials | Deferred to v0.2.0 |
| 2.1 Billing query | Done as `GET /v2/billing/costs` |
| 2.2 Pagination | Done with `cursor`, `has_next`, and one restart on 410 |
| 2.3 Retry | Done. Short retry for transport and 5xx. 429 is not retried |
| 3.1 ARN parsing | Done. Covers the Azure subscription id and the GCP project id. Bare AWS ids match by CZRN suffix |
| 3.2 Tag extraction | Done as configured `tag key=dimension ID` pairs |
| 3.3 Duplicate calls | Done in batch grouping and the month cache |
| 4.1 GetActualCost | Done |
| 4.2 Cache | Done. Required for the 60 request daily budget |
| 4.3 BatchCost | Done for the actual query type |
| 4.4 FOCUS 1.4 fields | Dropped. The documented response has none |
| 5.1 Error mapping | Done |
| 5.2 Validation | Done |
| 6.1 Coverage | Enforced by `make cover` in CI: 80% overall, 95% client and plugin |
| 6.2 Integration tests | Fake conformance is in `test/conformance`. The live test passed on 2026-11-18 |
| 6.3 Docs | README, CLAUDE.md, and docs/design.md record what the live check verified |
| 7.0 Release tooling | Release guards, Renovate, and workflows follow the family plugins. Renovate is onboarded |
| 7.1 Registry entry | Core PR rshade/finfocus#2200. A fresh `FINFOCUS_HOME` installs v0.1.0 with a verified checksum |
| 7.2 Release workflow | release-please. v0.1.0 has six archives and `checksums.txt` |

## Phase K (live CloudZero)

| Task | Status |
| --- | --- |
| K-1 Capture | Partly done on 2026-11-17. See the lists below |
| K-2 Fixtures | Done. Sanitized responses in `internal/plugin/testdata/live` and `internal/czclient/testdata/live` |
| K-3 Live integration test | Done. `TestLiveCosts` passed on 2026-11-18 |
| K-4 Verified wording | Done in README, CLAUDE.md, and docs/design.md |

Verified on the live account:

- Bare-key auth, the costs response shape, and multiple `group_by` values.
- The `Resource` dimension is one row per resource, valued as a CZRN.
- `CloudProvider` is `AWS`, and the filter is case-sensitive.
- A bad key returns 403 with `{"message": ...}`.
- The 60 request limit is a rolling 24 hours, shared with other callers of the
  key or organization. The 429 body says how many minutes to wait.

Still open:

- Azure and GCP `CloudProvider` spellings. This organization has AWS data only.
- Whether `end_date` is inclusive, and how fresh the newest day is. Both probes
  hit the rate limit.
- Currency of `cost` for a non-USD account.
- Whether `--adapter cloudzero` selects the plugin when it is installed from
  the registry.
