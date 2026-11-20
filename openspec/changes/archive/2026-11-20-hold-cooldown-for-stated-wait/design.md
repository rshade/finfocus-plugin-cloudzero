# Design

## Context

`Client` keeps two times after a 429 (`internal/czclient/client.go`):
`limitedUntil`, always `now + rateLimitCooldown` (5 minutes), which gates
budgeted requests, and `waitUntil`, `now + stated` when the body stated a
wait, which feeds only the error's `RetryAfter`. `statedWaitIn` returns zero
for a missing wait, zero minutes, or more than 1440 minutes. See proposal.md
for why the fixed gate is wrong.

## Goals / Non-Goals

**Goals:**

- One deadline decides both whether a request is sent and what its error says.

**Non-Goals:**

- Persisting the cooldown across processes (CONTEXT.md: no persistent state).
- Reading `Retry-After` or `X-RateLimit-*` headers, which CloudZero does not
  document.
- Sharing a cooldown between plugin processes that use the same key.

## Decisions

### The cooldown ends at `now + max(rateLimitCooldown, stated)`

`coolDown` sets `limitedUntil` from the longer of the floor and the stated
wait. `statedWaitIn` already bounds the stated wait to one rolling window, so
no new cap is needed.

Alternative: keep the 5-minute gate and only lengthen it when a second 429
arrives. Rejected: it spends a request at minute 5 to learn what CloudZero
already said.

### Replace `waitUntil` with a flag

With one deadline, the error's `RetryAfter` is `limitedUntil - now` whenever
the 429 stated a wait. `waitUntil` becomes a `bool` recording that a wait was
stated. A separate time would only diverge from `limitedUntil` for a stated
wait under 5 minutes, and there the old behavior named a time at which the
client still refused to send.

Alternative: keep `waitUntil` and report the remaining stated wait. Rejected:
after a 2-minute wait the error would read "retry later" from minute 2 to 5
while the client still refused, so the message would not match the behavior.

### A running cooldown only grows

Requests admitted before the first 429 returned can each draw their own 429.
`coolDown` keeps the later of the running and new end, with the flag that
belongs to it. Overwriting was harmless while every cooldown was 5 minutes;
with a stated wait, a second 429 stating less would end the cooldown early.

### A 429 without a stated wait keeps "retry later"

The client knows its own floor, but the message has always relayed only what
CloudZero said. Keeping "retry later" leaves `TestRateLimitCooldownSkipsRequests`
and the actual-cost error text unchanged.

## Risks / Trade-offs

- [A long stated wait blocks a process that could have recovered sooner, for
  example after another caller's window rolls off] → CloudZero stated the
  wait; honoring it is the point. A new process starts with no cooldown.
- [Wall-clock jumps shorten or lengthen the cooldown] → Unchanged from today;
  the client already compares against `c.now()`.
