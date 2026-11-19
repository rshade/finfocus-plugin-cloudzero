# Proposal

## Why

The plugin promises one CloudZero request per month and shared filter set, but
that only holds when requests arrive one at a time. FinFocus core calls
`GetActualCost` in parallel, so resources that share a cache key each miss the
cache and each send a request, and after a 429 every later resource asks again.
On 2026-11-18 one run of the `initial-30` stack (three filter groups) logged 35
rejected requests in a single process and returned 4 of its 8 resources. With
60 requests allowed in a rolling 24 hours, shared with every other caller of
the key, this makes the plugin the main consumer of its own limit. Issue #19.

## What Changes

- Concurrent requests that share a cache key (months, cost type, shared
  filters) make one CloudZero request. The first caller fetches; the others wait
  for its result. A waiter whose own context ends returns its context error and
  leaves the shared fetch running for the rest.
- A failed shared fetch returns its error to every waiter and is not cached, as
  today.
- After CloudZero answers 429, the client fails further requests with the
  rate-limit error for a 5-minute cooldown, without sending them and without
  spending budget.
- `BatchCost` gains the same deduplication, because it uses the same query path.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `actual-cost`: adds a requirement that concurrent requests sharing a cache key
  make one CloudZero request.
- `cloudzero-client`: adds a requirement that a 429 suppresses further requests
  for a cooldown.

## Impact

- Code: `internal/plugin/actual.go` (`costsFor`), `internal/czclient/client.go`
  (`get` and the 429 path), and tests run with `-race` in both packages.
- Behaviour: a run spends at most one request per cache key, plus nothing during
  a cooldown. Results are unchanged when CloudZero answers.
- No new dependency. No change to configuration or to the gRPC contract.
