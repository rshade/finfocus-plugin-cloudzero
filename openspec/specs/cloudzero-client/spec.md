# cloudzero-client Specification

## Purpose

Call the CloudZero REST API v2 within its documented contract and the 60
request rolling limit.

## Requirements

### Requirement: Requests carry the bare key and follow cursors

Every request SHALL send the API key as the bare `Authorization` value with no
`Bearer` prefix, and a `User-Agent` naming the plugin version. `GetCosts` SHALL
send repeated `group_by` and JSON `filters`, and SHALL follow `next_cursor`
while `has_next` is true. A repeated cursor SHALL fail as malformed.

#### Scenario: Two pages

- **WHEN** the first page has `has_next` true and `next_cursor` `page-2`
- **THEN** the client requests `cursor=page-2` and returns rows from both pages

Tested by: TestGetCostsPagesCursorAndHeaders, TestRepeatedCursorAndServerGiveUp, TestGetDimensions

### Requirement: Status codes map to fixed error kinds

HTTP 400 SHALL be a bad request, 401 and 403 an auth failure, 429 a rate limit,
and 5xx after retries unavailable. A 410 SHALL restart the query once. A body
that is not the documented JSON SHALL be malformed. Error text SHALL be fixed
per kind and SHALL NOT include the key, the query string, or the body. A 429
whose body says "Please wait N minute" SHALL carry N minutes as its wait, and
the text SHALL say "retry in N minutes" instead of "retry later".

#### Scenario: Captured live error bodies

- **WHEN** CloudZero returns the captured 429 or 403 body
- **THEN** the kinds are rate limit and auth, and the message does not repeat the body

#### Scenario: The 429 states a wait

- **WHEN** CloudZero returns the captured 429 body, which says to wait 24 minutes
- **THEN** the error carries a 24-minute wait and its text says "retry in 24 minutes"

#### Scenario: The 429 states no wait

- **WHEN** a 429 body has no parsable "Please wait N minute"
- **THEN** the error carries no wait and its text says "retry later"

<!-- markdownlint-disable-next-line MD013 -->
Tested by: TestGetCostsMapsStatuses, TestGetCostsRestartsOnceOn410, TestGetCostsMalformedBody, TestParseRejectsBadRows, TestGetDimensionsFailures, TestLiveErrorBodies, TestAPIErrorText, TestFloatValue, TestRateLimitWait

### Requirement: Short retry for transport errors and 5xx only

Transport errors and HTTP 5xx SHALL be retried, up to three attempts. HTTP 429
SHALL NOT be retried. A canceled context SHALL make no request.

#### Scenario: Server recovers on the third attempt

- **WHEN** the first two attempts return 502 and the third succeeds
- **THEN** three requests are made and the call succeeds

Tested by: TestGetCostsRetriesServerError, TestTransportRetry, TestCanceledContextDoesNotRetry

### Requirement: A per-process budget stops before the daily limit

The client SHALL count every first-page attempt, retries included, and SHALL
stop before `CLOUDZERO_DAILY_BUDGET` with a budget error. Cursor pages SHALL
NOT count. A missing key SHALL fail without a request.

#### Scenario: Retries stop at the budget

- **WHEN** the budget is 2 and the server always returns 502
- **THEN** two requests are made and the error is the budget kind

Tested by: TestBudgetStopsBeforeTheCall, TestRetriesStopAtTheBudget, TestMissingKeyDoesNotCall

### Requirement: The key never reaches a log

Log lines SHALL NOT contain the API key or the request URL. A transport error
SHALL be logged without its URL.

#### Scenario: Rejected key

- **WHEN** CloudZero answers 403 with the key echoed in its body
- **THEN** neither the error nor the log contains the key

Tested by: TestKeyNeverAppearsInLogs, TestTransportRetry

### Requirement: A rate limit suppresses further requests for a cooldown

After CloudZero answers HTTP 429, the client SHALL fail every further request
that spends budget (a first page, or a retry of one) with the rate-limit error,
without sending it and without spending budget, for the wait the 429 stated or
5 minutes, whichever is longer. Cursor pages, which CloudZero does not count,
SHALL still be sent. After the cooldown the client SHALL send requests again.
A 429 SHALL still never be retried.

#### Scenario: Requests during the cooldown

- **WHEN** a request gets a 429 that states no wait and a second request follows within 5 minutes
- **THEN** the second request returns the rate-limit error without an HTTP call
- **AND** the budget count does not change

#### Scenario: Requests after the cooldown

- **WHEN** a request follows more than 5 minutes after a 429 that states no wait
- **THEN** the client sends it to CloudZero

#### Scenario: A stated wait longer than 5 minutes

- **WHEN** CloudZero answered 429 saying to wait 24 minutes
- **THEN** a request 23 minutes 59 seconds later is not sent
- **AND** a request 24 minutes 1 second later is sent

#### Scenario: A stated wait shorter than 5 minutes

- **WHEN** CloudZero answered 429 saying to wait 2 minutes
- **THEN** a request 4 minutes 59 seconds later is not sent
- **AND** a request 5 minutes 1 second later is sent

#### Scenario: A stated wait outside one rolling window

- **WHEN** CloudZero answered 429 saying to wait 0 minutes or more than 1440 minutes
- **THEN** the cooldown lasts 5 minutes

#### Scenario: A second 429 during a cooldown

- **WHEN** a 429 stated 24 minutes, and an in-flight request's 429 a second later states less or no wait
- **THEN** the cooldown still ends 24 minutes after the first 429

#### Scenario: A cursor page during the cooldown

- **WHEN** a query already past its first page needs its next cursor page during a cooldown
- **THEN** the client sends the cursor page and the query completes

Tested by: TestRateLimitCooldownSkipsRequests, TestRateLimitCooldownLength, TestRateLimitCooldownKeepsLaterEnd, TestRateLimitCooldownStillSendsCursorPages

### Requirement: A cooldown error names what is left of the stated wait

When the 429 that started a cooldown stated a wait, every rate-limit error the
cooldown returns SHALL carry the time left until the cooldown ends, rounded up
to whole minutes in its text, so the message names when the client will send
again. A 429 that stated no wait SHALL leave cooldown errors saying "retry
later".

#### Scenario: A retry inside the cooldown

- **WHEN** CloudZero answered 429 saying to wait 24 minutes, and a request follows 90 seconds later
- **THEN** the request is not sent and its error says "retry in 23 minutes"

#### Scenario: A retry near the end of a long wait

- **WHEN** CloudZero answered 429 saying to wait 24 minutes, and a request follows 23 minutes 30 seconds later
- **THEN** the request is not sent and its error says "retry in 1 minute"

#### Scenario: A retry inside the floor after a short wait

- **WHEN** CloudZero answered 429 saying to wait 2 minutes, and a request follows 3 minutes later
- **THEN** the request is not sent and its error says "retry in 2 minutes"

Tested by: TestRateLimitCooldownKeepsStatedWait, TestRateLimitCooldownLength, TestAPIErrorText

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

### Requirement: Recommendations are read above a one-cent floor

`GetRecommendations` SHALL call `GET /v2/optimize/recommendations` with
`cost_impact_last_30_days[gte]=0.01` and the statuses `not_started`,
`in_progress`, and `ignored` as repeated `status` values, and follow
`next_cursor` while `has_next` is true. Its first page SHALL spend budget and respect the rate-limit cooldown
like a costs first page. Its cursor pages SHALL NOT spend budget.

#### Scenario: Query shape

- **WHEN** the client reads recommendations
- **THEN** the request carries the one-cent floor and the three statuses, and the budget count rises by one

#### Scenario: Two pages

- **WHEN** the first page has `has_next` true and `next_cursor` `page-2`
- **THEN** the client requests `cursor=page-2` and returns recommendations from both pages

#### Scenario: Recommendations during a cooldown

- **WHEN** a request got HTTP 429 less than 5 minutes ago
- **THEN** the recommendations read returns the rate-limit error without an HTTP call

Tested by: TestGetRecommendationsQueryAndPages, TestGetRecommendationsRespectsCooldown

### Requirement: Recommendation rows are parsed field by field

Each row SHALL yield its id, title, type id and name, effort, status,
resource, resource type and name, cost impact, created time (Unix seconds),
and the account, region, and cloud provider lists. A null list SHALL be
empty. A row without an id or resource SHALL be dropped. A body that is not
the documented JSON SHALL be malformed.

#### Scenario: The captured live response

- **WHEN** the client parses the sanitized 2026-11-19 response
- **THEN** it returns five rows whose resources are volume CZRNs and whose cost impacts are above zero

#### Scenario: A row without a resource

- **WHEN** a row has a null `resource`
- **THEN** it is dropped and the other rows are returned

Tested by: TestParseRecommendations
