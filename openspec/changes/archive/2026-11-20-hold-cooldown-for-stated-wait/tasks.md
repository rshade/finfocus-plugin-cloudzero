# Tasks

## 1. Hold the cooldown for the stated wait

- [x] 1.1 Add `TestRateLimitCooldownLength` in `internal/czclient/client_test.go`:
      a table of 429 bodies (no wait, 0 minutes, 2000 minutes, 2 minutes,
      24 minutes) with the expected cooldown and the error text at a time
      inside it. A request one second before the end is not sent; one
      second after is sent. Verify it fails against the current code
- [x] 1.2 Extend `TestRateLimitCooldownKeepsStatedWait` with requests at
      5 minutes 1 second and 23 minutes 30 seconds that are not sent and say
      the texts "retry in 19 minutes" and "retry in 1 minute" respectively.
      Verify it fails against the current code
- [x] 1.3 In `internal/czclient/client.go`, set `limitedUntil` to
      `now + max(rateLimitCooldown, stated)` in `coolDown`, replace
      `waitUntil` with a flag that a wait was stated, return
      `limitedUntil - now` from `coolingDown` when it is set, and update the
      `rateLimitCooldown` comment. Verify with `go test ./internal/czclient/`
- [x] 1.4 Update the cooldown text in `README.md`, the rate-limit notes in
      `CLAUDE.md`, and `docs/design.md` where it describes the cooldown.
      Verify with `mise exec -- markdownlint-cli2` on the changed files
- [x] 1.5 Keep the later end when a second 429 arrives during a cooldown, so
      a request already in flight cannot shorten it (found in local review).
      Verify with `TestRateLimitCooldownKeepsLaterEnd`

## 2. Integration

- [x] 2.1 Run `make validate` and
      `mise exec -- openspec validate hold-cooldown-for-stated-wait --strict`.
      Verify both pass

## Workflow follow-up

- Archive the change in the same commit as the code, with `Closes #26`.
- Move #26 to the 2026-Q4 completed list in `ROADMAP.md`.
