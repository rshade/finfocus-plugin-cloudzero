# Proposal

## Why

When CloudZero answers HTTP 429, its body says how long to wait: "Please wait
24 minute(s) before trying again." The plugin returns a fixed message, "retry
later", and drops that number, so a user who hits the limit has to guess when
the rolling 24-hour window will have room. Issue #11.

## What Changes

- On a 429, the client reads only the integer from `Please wait N minute` in
  the body and records it on the error as `RetryAfter`.
- The rate-limit message says "retry in N minutes" instead of "retry later"
  when the number was found, and is unchanged when it was not.
- Nothing else from the body reaches the message. No retry or sleep is added,
  and the 5-minute cooldown keeps its length.
- Errors the cooldown returns carry what is left of the stated wait. Core
  retries `ResourceExhausted` within about two seconds and keeps the last
  error, which is a cooldown error, so without this the user would still
  read "retry later".
- The wait goes into the message only, not into `ErrorDetail.retry_after_seconds`:
  core treats that field as a retry delay and would sleep for the whole wait.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `cloudzero-client`: a 429's error carries the wait CloudZero states.
- `actual-cost`: the `ResourceExhausted` message names that wait.

## Impact

- Code: `internal/czclient/errors.go` (`APIError.RetryAfter`, `Error()`),
  `internal/czclient/client.go` (the non-200 path of `pages` and
  `GetDimensions`), and tests.
- Behaviour: only the text of a rate-limit error changes.
- No new dependency. No change to configuration or to the gRPC contract.
