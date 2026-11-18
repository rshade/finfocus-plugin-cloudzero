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
per kind and SHALL NOT include the key, the query string, or the body.

#### Scenario: Captured live error bodies

- **WHEN** CloudZero returns the captured 429 or 403 body
- **THEN** the kinds are rate limit and auth, and the message does not repeat the body

<!-- markdownlint-disable-next-line MD013 -->
Tested by: TestGetCostsMapsStatuses, TestGetCostsRestartsOnceOn410, TestGetCostsMalformedBody, TestParseRejectsBadRows, TestGetDimensionsFailures, TestLiveErrorBodies, TestAPIErrorText, TestFloatValue

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
