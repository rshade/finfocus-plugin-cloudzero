# Design

## Context

`costsFor` (`internal/plugin/actual.go`) checks the cache, then calls
`CostSource.GetCosts`, then fills the cache. Nothing tracks a fetch in flight,
so concurrent callers with the same cache key all fetch. In `czclient`, `get`
spends budget and sends the request; `pages` turns a non-200 status into an
`APIError`, and a 429 becomes `KindRateLimit`. Nothing remembers the 429. See
proposal.md for the measured impact.

## Goals / Non-Goals

**Goals:**

- One CloudZero request per cache key while one is in flight.
- No request, and no budget spent, during a cooldown after a 429.
- Deterministic tests under `-race`, with no sleeping on wall-clock time.

**Non-Goals:**

- Deduplicating across processes. Each plugin process keeps its own state.
- Parsing the wait time from the 429 body; that is #11.

## Decisions

### Deduplicate in the plugin, keyed by the existing cache key

The plugin already computes the cache key (months, cost type, shared filters),
and that key is exactly what "the same query" means. A small `inflight` type in
`internal/plugin` holds `map[string]*call` under a mutex. A `call` has a `done`
channel, the rows, and the error. The first caller for a key creates the call
and fetches; later callers wait on `done` or on their own `ctx.Done()`.

- *Alternative: `golang.org/x/sync/singleflight`.* It does the same job but adds
  a module, and its `DoChan` still needs our own context handling. About thirty
  lines of our own code avoid the dependency.
- *Alternative: a lock around the whole of `costsFor`.* Simpler, but it
  serializes unrelated keys, so the second account waits on the first account's
  fetch.

### The shared fetch runs on a context detached from any one caller

If the first caller's context were used and that caller gave up, every waiter
would fail. The fetch therefore uses `context.WithoutCancel(ctx)`, which keeps
values such as trace ids but not cancellation, and re-applies the first
caller's deadline when it has one (`detached`). The client's 30-second timeout
bounds one HTTP attempt, not the fetch: a month of cursor pages with retries
could run far longer, so the deadline is what bounds it. A caller whose
context has already ended starts no fetch. A waiter that gives up returns
`ctx.Err()`, which is mapped like any other cancellation.

### Errors are shared, not cached

The completed `call` is removed from the map before `done` is closed, so a
request after a failure starts a new fetch. Only success fills the cache,
as today. The rate-limit cooldown below covers the case where a new fetch
would only produce another 429.

### The cooldown lives in the client

The 429 is a property of the key or organization, not of one query, so the
client is the right owner. `Client` gains `limitedUntil time.Time` under its
existing mutex and an injectable `now func() time.Time` (as `sleep` already is).
`get` checks the cooldown before each attempt that spends budget, and sets it
on any 429. Cursor pages spend no budget and CloudZero does not count them, so
they are still sent: a query part-way through its pages finishes rather than
throwing away a first page it has paid for.

- *Assumption:* a fixed 5-minute cooldown. A CLI run lasts seconds, so it sees
  one 429 at most. A long-running process recovers within 5 minutes. #11 can
  replace the fixed value with the wait time in the 429 body.

## Risks / Trade-offs

- [A slow shared fetch holds every waiter] → bounded by the first caller's
  deadline; waiters can still leave on their own context. A first caller with
  no deadline leaves it unbounded, as before this change.
- [A 5-minute cooldown can outlast a shorter real window] → the cost is a
  delayed retry, not a wrong result; #11 refines it.
- [`context.WithoutCancel` needs Go 1.21 or later] → the module requires Go
  1.27.
