# `finfocus-plugin-cloudzero` Strategic Roadmap

## Vision

Give FinFocus users the cost CloudZero has actually billed for each resource in
a stack, from a few CloudZero requests per stack rather than one per resource,
and never a guessed or placeholder number.

This project follows the **OpenSpec** change process (`openspec/specs/`).
See [CONTEXT.md](./CONTEXT.md) for architectural boundaries. v0.1.0 followed
[plan.md](./plan.md). Its task record, TASKS.md, was folded into this roadmap.

Most items below are not yet filed as GitHub issues. Items marked
*(not filed)* need an issue before `/roadmap sync` can track them.

---

## Immediate Focus (v0.1.1)

**Goal:** ship the first patch release and close the live-check questions that
only need a couple of CloudZero calls.

- Release PR 3 (0.1.1): the rate-limit message drops "retry tomorrow" and now
  describes the limit as 60 cost requests in a rolling 24 hours. Merge the
  Renovate update for `google.golang.org/grpc` v1.85.0 first if it should ship
  in the same release. [S]
- Settle whether `end_date` is inclusive and how fresh the newest day is. Two
  calls, blocked on 2026-11-17 by the rolling rate limit. *(not filed)* [S]
- Confirm `--adapter cloudzero` selects the plugin when it is installed from the
  registry. One live `finfocus cost actual` run. *(not filed)* [S]

## Near-Term Vision (v0.2.0)

**Goal:** let core supply the CloudZero key, and attribute the costs CloudZero
bills to parts of a stack resource.

- **Per-request credentials and settings advertisement.** Accept the key from
  `x-finfocus-credential-*` metadata and advertise settings (`finfocus-spec`
  v0.7.6), so core can deliver the key from ESC and show it in the dashboard.
  Deferred from v0.1.0 by the owner. *(not filed)* [L]
- **Root volume cost on the instance.** CloudZero bills an instance's root EBS
  volume as a separate `volume` CZRN, so the instance result omits it. Pulumi
  state carries the root volume id, so the plugin can add it to the instance
  result as a breakdown line. It stays a billed cost of the same resource, not
  a slice. *(not filed)* [M]
- **Rate-limit wait time.** The 429 body states how many minutes to wait.
  Return that number in the `ResourceExhausted` message without forwarding the
  body. *(not filed)* [S]

## Future Vision (Long-Term)

- **Azure and GCP verification.** The owner's CloudZero organization has AWS
  data only, so the `CloudProvider` matching and CZRN shapes for Azure and GCP
  are untested. Needs an organization with Azure or GCP billing. Once the CZRN
  format is known, convert Azure resource ids and GCP resource names the way
  AWS ARNs are converted today. *(not filed)* [L]
- **Non-USD currency.** The costs response carries no currency. Check what a
  non-USD organization returns before trusting `CLOUDZERO_CURRENCY`.
  *(not filed)* [S]
- **Large-account paging.** A month query for a big account returns every
  resource in the account and region. Pages are free, but the client stops at
  100 pages of 10,000 rows. Measure a large account before raising the cap.
  *(not filed)* [M]
- **Elastic IP charges.** CloudZero bills an Elastic IP's public IPv4 charge on
  its network interface. Decide whether the EIP result should include it through
  the association of the EIP. *(not filed)* [M]

## Decisions Carried From v0.1.0

The owner accepted these defaults on 2026-11-15. Changing one is a roadmap
decision, not a bug fix.

- Ship tested against fakes, then verify live. The live check ran on
  2026-11-17 against the `initial-30` `dev` stack.
- Decline a resource when no resource dimension is configured.
- Default cost type is `real_cost`.
- Per-request credentials and settings advertisement wait for v0.2.0.
- Pin `finfocus-spec` v0.7.12, core's version.

## Completed Milestones

### 2026-Q4

- [x] `release`: v0.1.0 published with six archives and checksums. Closed 2026-11-18. [M]
- [x] `registry`: added to the FinFocus registry (`rshade/finfocus` PR 2200). Closed 2026-11-18. [S]
- [x] `mapping`: AWS resources without an ARN match by CZRN suffix. Closed 2026-11-18. [M]
- [x] `mapping`: CloudProvider matched from row values, not a table. Closed 2026-11-18. [M]
- [x] `test`: live check on initial-30 and sanitized response fixtures. Closed 2026-11-18. [M]
- [x] `process`: OpenSpec adopted for behavior changes. Closed 2026-11-18. [S]
- [x] `openspec`: baseline specs for seven capabilities, linked to tests. Closed 2026-11-18. [M]

## Boundary Safeguards

These come from [CONTEXT.md](./CONTEXT.md). A roadmap item that needs one of
them is redesigned, not shipped.

- **No projected cost, pricing spec, or estimate.** A CloudZero forecast, if
  one is ever exposed, is still a prediction. It belongs in a different plugin
  or a deliberate change to CONTEXT.md, not in this roadmap.
- **No slice cost attributed to a resource.** The root volume and Elastic IP
  work in this roadmap qualifies only because the cost is billed to a part of the same
  resource.
- **No per-resource queries and no 429 retries.** Every feature must keep a
  stack at a few CloudZero requests.
- **No hardcoded CloudZero dimension IDs or spellings.**
- **No writes to CloudZero or to a cloud provider.**
- **No persistent storage.**
- **No secrets or upstream bodies in output.**
- **No live calls from CI or agent sessions without the owner's go-ahead.**
