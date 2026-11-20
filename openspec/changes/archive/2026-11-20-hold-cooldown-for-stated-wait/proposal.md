# Proposal

## Why

After a 429 the client stops budgeted requests for a fixed 5 minutes, but
CloudZero's 429 body often states a longer wait ("Please wait 24 minute(s)
before trying again"). At minute 5 the plugin starts sending again into a
window CloudZero said is still full, and each request draws another 429
against a limit shared with other callers. A long-lived plugin process (`finfocus --web`,
repeated `cost history collect-daily`) pays for it. Issue #26.

## What Changes

- The cooldown after a 429 lasts for the wait CloudZero stated, with 5 minutes
  as the floor. A 429 that states no wait, or an invalid one, keeps 5 minutes.
- The stated wait stays bounded by one rolling window: the parser already
  rejects zero and anything over 1440 minutes.
- A cooldown error names the time left until the client sends again, so the
  message and the behavior agree. A 429 that stated no wait still gives the
  text "retry later" in its errors.
- Cursor pages are still sent during the cooldown. No retry on 429 is added,
  and nothing is persisted across processes.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `cloudzero-client`: "A rate limit suppresses further requests for a
  cooldown" takes its length from the stated wait, with a 5-minute floor.
  "A cooldown error names what is left of the stated wait" names the time
  left in the cooldown.

## Impact

- Code: `internal/czclient/client.go` (`coolDown`, `coolingDown`, the
  `rateLimitCooldown` comment) and `internal/czclient/client_test.go`.
- Docs: `README.md`, `CLAUDE.md`, `docs/design.md`.
- Behavior: fewer requests reach CloudZero after a 429 that states a long
  wait. No change to configuration, the gRPC contract, or dependencies.
