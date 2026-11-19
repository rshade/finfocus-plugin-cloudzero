# actual-cost Specification

## Purpose

Return the cost CloudZero has billed for one resource, per day, without
spending one CloudZero request per resource.

## Requirements

### Requirement: One month-aligned query serves every resource that shares filters

`GetActualCost` SHALL query the UTC months that cover the request window,
grouped by the resource dimension and `CloudProvider`, and filtered only on
shared dimensions (`Account`, `Region`, tag dimensions). It SHALL NOT filter
on the resource or on `CloudProvider`. A second request with the same months,
cost type, and shared filters SHALL be answered from the cache.

#### Scenario: Two resources in one account share one call

- **WHEN** two instances in the same account and region are requested for March
- **THEN** CloudZero is called once, with filters `Account` and `Region` only
- **AND** each response holds only that instance's rows

#### Scenario: A repeated request hits the cache

- **WHEN** the same request is made twice
- **THEN** CloudZero is called once

Tested by: TestGetActualCostSlicesMonthAndCaches, TestGetActualCostSharesOneCallAcrossResources

### Requirement: Results are the billed days inside the window

The response SHALL hold one result per CloudZero row that belongs to the
resource and whose day overlaps the requested window, in date order. `source`
SHALL be `cloudzero:<cost type> <currency>`. A resource matched by CZRN suffix
SHALL be answered the same way. No rows SHALL be an empty result with
`FALLBACK_HINT_RECOMMENDED`, never a `$0` row.

#### Scenario: Days outside the window are dropped

- **WHEN** rows exist for March 1, 15, and 20 and the window is March 10 to 16
- **THEN** one result is returned, for March 15

#### Scenario: No rows means no data

- **WHEN** CloudZero returns no rows for the resource
- **THEN** the result list is empty and the fallback hint is `RECOMMENDED`

Tested by: TestGetActualCostSlicesMonthAndCaches, TestGetActualCostMatchesResourceWithoutARN, TestGetActualCostEmptyIsNoData

### Requirement: The plugin computes expires_at

CloudZero sends no TTL. A result for a day on or after yesterday (UTC) SHALL
expire after `CLOUDZERO_RECENT_TTL`. An older day SHALL expire after
`CLOUDZERO_CACHE_TTL`.

#### Scenario: Recent and old days get different TTLs

- **WHEN** rows exist for March 1 and March 20 and now is March 20
- **THEN** the March 1 result expires after the cache TTL and the March 20 result after the recent TTL

Tested by: TestGetActualCostRecentDayUsesShortTTL

### Requirement: Bad requests fail before any CloudZero call

A nil request, a missing or empty window, or a resource that yields no CZRN
SHALL return `InvalidArgument`. A missing API key or resource dimension SHALL
return `FailedPrecondition`. None of these SHALL call CloudZero.

#### Scenario: Missing key

- **WHEN** no API key is configured
- **THEN** the status is `FailedPrecondition` and CloudZero is not called

Tested by: TestGetActualCostRejectsBadRequests

### Requirement: Rate limit and budget become ResourceExhausted without leaking the key

An HTTP 429 or an exhausted request budget SHALL return `ResourceExhausted`
with a fixed message. No error message or log line SHALL contain the API key
or the upstream response body.

#### Scenario: CloudZero returns 429

- **WHEN** CloudZero answers 429 with a body
- **THEN** the status is `ResourceExhausted` and the message does not contain the body

#### Scenario: Auth failure does not leak the key

- **WHEN** CloudZero answers 403
- **THEN** the status is `PermissionDenied` and neither the error nor the logs contain the key

Tested by: TestGetActualCostRateLimitAndBudget, TestKeyNeverLogged

### Requirement: Concurrent requests that share a query make one CloudZero call

When several `GetActualCost` or `BatchCost` requests need the same month query
(same months, cost type, and shared filters) at the same time, the plugin SHALL
send one CloudZero request and give its result to all of them. A request whose
own context ends while waiting SHALL return its context error without stopping
the shared request. A failed shared request SHALL return its error to every
waiter and SHALL NOT be cached.

#### Scenario: Concurrent resources in one account

- **WHEN** twenty concurrent requests ask for instances in the same account and region for March
- **THEN** CloudZero is called once and every request gets its own instance's rows

#### Scenario: A waiter gives up

- **WHEN** a request's context is canceled while it waits for a shared CloudZero call
- **THEN** that request returns promptly with its context error
- **AND** the shared call completes and serves the other waiters

#### Scenario: The shared call fails

- **WHEN** the shared CloudZero call fails
- **THEN** every waiter receives the mapped error and the next request calls CloudZero again

Tested by: TestGetActualCostConcurrentRequestsShareOneCall, TestGetActualCostWaiterHonorsItsContext, TestGetActualCostSharedFailureIsNotCached

### Requirement: A shared CloudZero call is bounded by its first caller

A request whose context has already ended SHALL NOT start a CloudZero request.
A shared request SHALL ignore its first caller's cancellation but SHALL keep
that caller's deadline, so it cannot outlive the time the caller allowed.

#### Scenario: The caller has already given up

- **WHEN** a request arrives with a canceled context and no shared call is in flight
- **THEN** it returns its context error and CloudZero is not called

#### Scenario: The first caller is canceled

- **WHEN** the caller that started a shared call is canceled before its deadline
- **THEN** the shared call keeps running with that deadline

Tested by: TestGetActualCostCanceledCallerStartsNoFetch, TestDetachedKeepsDeadlineNotCancel
