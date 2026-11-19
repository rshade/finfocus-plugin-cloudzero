# Design

## Context

`pages` and `GetDimensions` turn every non-200 status into
`newAPIError(statusKind(code), code)` and drop the body. `statusFrom` in
`internal/plugin/status.go` sends `api.Error()` to core, so whatever
`Error()` returns is what the user sees. The captured live 429 body is in
`internal/czclient/testdata/live/rate_limit_429.json`.

## Goals / Non-Goals

**Goals:** tell the user how long CloudZero asked them to wait, without
forwarding any other part of the body.

**Non-Goals:** retrying or sleeping on 429 (CONTEXT.md Hard No 3);
`Retry-After` headers, which CloudZero does not document; changing the
cooldown.

## Decisions

### One helper builds every non-200 error

`responseError(code, body)` replaces the two `newAPIError(statusKind(code),
code)` calls. For 429 it matches `Please wait (\d+) minute` and sets
`RetryAfter` to that many minutes when the number parses as a positive
integer. Only the parsed integer leaves the function, so the body still
never reaches a message.

### The message replaces "retry later"

With `RetryAfter` set, `Error()` returns "CloudZero allows 60 cost requests in
a rolling 24 hours; retry in 24 minutes" ("1 minute" when singular). Appending
to "retry later" would read twice. Without it the text is unchanged.

### The wait stays out of `ErrorDetail`

Core's `retryHint` reads `ErrorDetail.retry_after_seconds` as the delay before
its next attempt and does not cap it. Setting it would make one `cost actual`
run sleep for 24 minutes. The message is enough for a person to act on.

### Cooldown rejections carry what is left of the wait

Core retries `ResourceExhausted` up to three times within about two seconds
and reports the last error. Those retries land inside the plugin's cooldown,
so the error the user sees comes from `admit`, not from the 429. The client
therefore records when the stated wait ends (`waitUntil`) beside
`limitedUntil`, and a cooldown error carries the time left. The text rounds
up to whole minutes, so 23 minutes 30 seconds reads "retry in 24 minutes".
A 429 that stated no wait leaves cooldown errors saying "retry later".

## Risks / Trade-offs

- [CloudZero rewords the body] → no match, so the message falls back to
  today's text; a test covers that.
- [A wait in hours] → the regex accepts only minutes; anything else falls back.
- [A huge or bogus number] → a wait over 1440 minutes, one rolling window,
  is ignored, so the message never reports a wrapped or impossible wait.
