# Spec Delta

## ADDED Requirements

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
