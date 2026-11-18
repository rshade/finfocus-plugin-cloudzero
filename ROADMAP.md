# `finfocus-plugin-cloudzero` Strategic Roadmap

## Vision

Give FinFocus users the cost CloudZero has actually billed for each resource in
a stack, from a few CloudZero requests per stack rather than one per resource,
and never a guessed or placeholder number.

This project follows the **OpenSpec** change process (`openspec/specs/`).
See [CONTEXT.md](./CONTEXT.md) for architectural boundaries. v0.1.0 followed
[plan.md](./plan.md). Its task record, TASKS.md, was folded into this roadmap.

Each planned item is a GitHub issue. Releases are cut by the owner and are not
tracked here. Spikes are sized by timebox, not effort: their deliverable is a
decision recorded in the docs.

---

## Immediate Focus

**Goal:** close the live-check questions that only need a couple of CloudZero
calls.

- [ ] #7 Spike: settle `end_date` inclusivity and CloudZero data freshness
  *(2-hour timebox)*
- [ ] #8 Spike: does `--adapter cloudzero` select the plugin? *(2-hour timebox)*

Release PR 3 (0.1.1) is open and held by the owner.

## Near-Term Vision

**Goal:** let core supply the CloudZero key, and attribute the costs CloudZero
bills to parts of a stack resource.

- [ ] #9 Accept the CloudZero key per request and advertise plugin settings [L]
- [ ] #10 Include the root EBS volume cost in an instance's actual cost [M]
- [ ] #11 Report the CloudZero rate-limit wait time in the `ResourceExhausted`
  message [S]

## Future Vision (Long-Term)

- [ ] #12 Spike: verify Azure and GCP resources against CloudZero data
  *(watch: needs an organization with Azure or GCP data)*
- [ ] #13 Spike: confirm the currency of CloudZero costs for a non-USD
  organization *(watch: needs a non-USD organization)*
- [ ] #14 Spike: measure month-query paging on a large CloudZero account
  *(watch: needs a large account)*
- [ ] #15 Attribute the public IPv4 charge of an Elastic IP to the EIP [M]

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
