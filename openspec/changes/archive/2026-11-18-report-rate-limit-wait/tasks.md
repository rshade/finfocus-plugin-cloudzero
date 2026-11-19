# Tasks

## 1. Carry the wait on the error

- [x] 1.1 Add `RetryAfter time.Duration` to `APIError` in
      `internal/czclient/errors.go`. `Error()` for `KindRateLimit` says "retry in
      N minutes" ("1 minute" when singular) when it is set, and "retry later"
      otherwise. Verify with `TestAPIErrorText`
- [x] 1.2 Add `responseError(code, body)` in `internal/czclient`, used by `pages`
      and `GetDimensions` for every non-200 status. For 429 it parses only the
      integer from `Please wait (\d+) minute`, ignoring a wait over 1440 minutes. Verify with `TestRateLimitWait`
      (captured body gives 24 minutes; no number, zero, and malformed bodies give
      none) and `TestLiveErrorBodies` (the message holds no body text)

- [x] 1.3 Record the stated wait when a 429 starts the cooldown (`waitUntil`),
      and give cooldown errors the time left, rounded up in the text. Verify
      with `TestRateLimitCooldownKeepsStatedWait`

## 2. Reach the user

- [x] 2.1 Add `TestGetActualCostRateLimitNamesWait` in `internal/plugin`: a
      `GetCosts` error with a 24-minute wait becomes `ResourceExhausted` with
      "retry in 24 minutes". Verify with `go test ./internal/plugin/`
- [x] 2.2 Update the 429 row in `docs/design.md`, the rate-limit notes in
      `CLAUDE.md` and `README.md`. Verify with markdownlint
- [x] 2.3 Run `make validate` and
      `openspec validate report-rate-limit-wait --strict`. Verify both pass
