# Tasks

## 1. One shared fetch per cache key

- [x] 1.1 Add an optional gate to `fakeCosts` in `internal/plugin/plugin_test.go`
      (a channel the fake waits on before answering, and a count of calls
      started), so tests can hold a fetch open while other requests arrive.
      Verify existing plugin tests still pass with `go test ./internal/plugin/`
- [x] 1.2 Add an `inflight` type in `internal/plugin` (mutex, `map[string]*call`,
      `call` with `done` channel, rows, and error) and use it in `costsFor`: on a
      cache miss, the first caller for a key fetches with
      `context.WithoutCancel(ctx)` and fills the cache on success; other callers
      wait on `done` or their own `ctx.Done()`. Remove the call from the map before
      closing `done`. Verify with `TestGetActualCostConcurrentRequestsShareOneCall`
      (twenty concurrent requests, one `GetCosts` call, each request gets its own
      rows) under `go test -race ./internal/plugin/`
- [x] 1.3 Add `TestGetActualCostWaiterHonorsItsContext` (a waiter whose context is
      canceled returns `codes.Canceled` promptly while the gated fetch is held;
      after releasing the gate, another waiter gets rows) and
      `TestGetActualCostSharedFailureIsNotCached` (a failed shared fetch reaches
      every waiter; the next request calls `GetCosts` again). Verify both pass
      under `-race`

## 2. Cooldown after a rate limit

- [x] 2.1 Give `Client` in `internal/czclient/client.go` a `limitedUntil` time under
      its mutex and an injectable `now` clock (defaulting to `time.Now`, as `sleep`
      defaults to `time.Sleep`). In `get`, return the rate-limit error before
      spending budget while `now` is before `limitedUntil`; in `pages`, set
      `limitedUntil` to now plus 5 minutes when a request returns 429. Verify with
      `TestRateLimitCooldownSkipsRequests` (429, then a second call with no HTTP
      request and an unchanged `RequestsUsed`, then a call after advancing the
      clock past 5 minutes that does reach the server)
- [x] 2.2 Update README.md "CloudZero call shape" and CLAUDE.md "Rate Limiting" to
      state the shared fetch and the 5-minute cooldown. Verify with markdownlint
      and Vale on the changed lines

## 3. Integration check

- [x] 3.1 Run `make validate` and `go test -race ./...`, and
      `openspec validate dedupe-concurrent-cost-queries --strict`. Verify all pass
      and `make cover` keeps its thresholds

## Workflow follow-up

- Archive the change in the same commit as the code.
- Rebase `fix/name-cloudzero` onto this branch and rerun its task 2.2 live check
  once the CloudZero window has room.
- Close #19 through the PR.
