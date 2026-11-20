# Spec Delta

## ADDED Requirements

### Requirement: Budgets are read with their current spend

`GetBudgets` SHALL call `GET /v2/budgets` with `expand=current` and follow
`next_cursor` while `has_next` is true, returning every budget with its id,
name, view id, monthly planned limits, alerts, and current planned and
actual. Its first page SHALL spend budget and respect the rate-limit
cooldown like a costs first page. Its cursor pages SHALL NOT spend budget.

#### Scenario: Two pages of budgets

- **WHEN** the first page has `has_next` true and `next_cursor` `page-2`
- **THEN** the client requests `cursor=page-2`, returns budgets from both pages, and the budget count rises by one

#### Scenario: Budgets during a cooldown

- **WHEN** a request got HTTP 429 less than 5 minutes ago
- **THEN** the budgets read returns the rate-limit error without an HTTP call

Tested by: TestGetBudgetsPagesAndCharges, TestGetBudgetsRespectsCooldown

### Requirement: Budget amounts are parsed as numbers

Planned amounts and the current planned and actual SHALL be parsed from a
JSON number or a numeric string. A budget with an amount that is neither
SHALL keep the other fields and leave that amount unset. A body that is not
the documented JSON SHALL be malformed.

#### Scenario: Current spend as a string

- **WHEN** a budget's `current.actual` is `"120.50"`
- **THEN** the parsed actual is 120.5

#### Scenario: Body is not JSON

- **WHEN** the budgets body is not JSON
- **THEN** the error kind is malformed

Tested by: TestParseBudgets
