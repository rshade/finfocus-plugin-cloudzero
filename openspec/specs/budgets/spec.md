# budgets Specification

## Purpose

List the budgets kept in CloudZero as FinFocus budgets, with the spend
CloudZero has recorded against them, so core can judge their health.

## Requirements

### Requirement: Every CloudZero budget is listed with its monthly limit

`GetBudgets` SHALL return one budget per CloudZero budget, with CloudZero's id
and name, source `cloudzero`, a monthly period, and as its limit the planned
amount for the current UTC month in `CLOUDZERO_CURRENCY`. A budget whose
granularity is set and is not monthly, or that has no planned amount above
zero for that month, SHALL be left out and counted in a debug log. A missing
granularity SHALL count as monthly. The view id SHALL be kept in metadata, and
the budget filter SHALL be empty.

#### Scenario: A budget with a planned amount this month

- **WHEN** CloudZero lists a budget planned at 500 for the current month
- **THEN** the response has that budget with a monthly limit of 500 in the configured currency and source `cloudzero`

#### Scenario: A budget with nothing planned this month

- **WHEN** a budget's planned limits have no entry for the current month
- **THEN** the response leaves that budget out

#### Scenario: A quarterly budget

- **WHEN** a budget's granularity is `quarterly`
- **THEN** the response leaves that budget out

#### Scenario: A budget without a granularity

- **WHEN** a budget has no `granularity` and plans 300 for the current month
- **THEN** the response has that budget with a monthly limit of 300

Tested by: TestGetBudgetsMapsCloudZeroBudgets, TestGetBudgetsSkipsUnplannedMonth

### Requirement: Budget status carries spend and nothing derived

Each budget SHALL carry a status whose current spend is CloudZero's actual for
the month and whose currency is `CLOUDZERO_CURRENCY`, whatever
`include_status` says, because core computes health only when status is
present. The plugin SHALL NOT set forecasted spend, percentages, or health.
Core derives them.

#### Scenario: Status without include_status

- **WHEN** core sends an empty `GetBudgetsRequest` and CloudZero reports an actual of 120.5
- **THEN** the budget's status has current spend 120.5 and no forecast or health

#### Scenario: A missing or unparsable actual

- **WHEN** CloudZero omits `current` for a budget, or its actual is not a number
- **THEN** the budget is still listed, without a status

Tested by: TestGetBudgetsStatusIsSpendOnly

### Requirement: Alert percentages become actual-spend thresholds

Each CloudZero alert percentage that is enabled SHALL become a threshold of
that percentage and type actual, in ascending order. An alert that is not
enabled, or whose key is not a number from 1 to 100, SHALL be left out.

#### Scenario: Two alerts, one disabled

- **WHEN** a budget's alerts are `{"80": "Y", "100": "N"}`
- **THEN** the budget has one threshold, 80 percent, type actual

Tested by: TestGetBudgetsThresholds

### Requirement: The summary counts the budgets returned

The response SHALL carry a summary whose total is the number of budgets
returned. Health counts SHALL be zero, because core computes health.

#### Scenario: No budgets

- **WHEN** CloudZero has no budgets
- **THEN** the response has no budgets and a summary with a total of 0

Tested by: TestGetBudgetsSummary

### Requirement: Budgets are read once per cache window

Budgets SHALL be fetched with one CloudZero query, cursor pages included, and
cached for `CLOUDZERO_RECENT_TTL`. Concurrent requests SHALL share one fetch.
A failed fetch SHALL NOT be cached and SHALL map to the same status codes as
cost queries. A missing API key SHALL fail with `FailedPrecondition` without
a call.

#### Scenario: Two requests inside the window

- **WHEN** core calls `GetBudgets` twice within `CLOUDZERO_RECENT_TTL`
- **THEN** CloudZero is called once

#### Scenario: CloudZero rate limit

- **WHEN** the budgets query gets HTTP 429
- **THEN** `GetBudgets` returns `ResourceExhausted` and nothing is cached

Tested by: TestGetBudgetsCachesOneFetch, TestGetBudgetsErrors
