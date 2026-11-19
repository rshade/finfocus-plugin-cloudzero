# Spec Delta

## MODIFIED Requirements

### Requirement: Rate limit and budget become ResourceExhausted without leaking the key

An HTTP 429 or an exhausted request budget SHALL return `ResourceExhausted`
with a fixed message. A 429 that stated a wait SHALL name it in the message.
No error message or log line SHALL contain the API key or the upstream
response body.

#### Scenario: CloudZero returns 429

- **WHEN** CloudZero answers 429 with a body
- **THEN** the status is `ResourceExhausted` and the message does not contain the body

#### Scenario: CloudZero states a wait

- **WHEN** CloudZero answers 429 saying to wait 24 minutes
- **THEN** the `ResourceExhausted` message says "retry in 24 minutes"

#### Scenario: Auth failure does not leak the key

- **WHEN** CloudZero answers 403
- **THEN** the status is `PermissionDenied` and neither the error nor the logs contain the key

Tested by: TestGetActualCostRateLimitAndBudget, TestKeyNeverLogged, TestGetActualCostRateLimitNamesWait
