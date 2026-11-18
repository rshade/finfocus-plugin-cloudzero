# Spec Delta

## ADDED Requirements

### Requirement: A rate limit suppresses further requests for a cooldown

After CloudZero answers HTTP 429, the client SHALL fail every further request
with the rate-limit error for 5 minutes, without sending it and without
spending budget. After the cooldown the client SHALL send requests again. A
429 SHALL still never be retried.

#### Scenario: Requests during the cooldown

- **WHEN** a request gets 429 and a second request follows within 5 minutes
- **THEN** the second request returns the rate-limit error without an HTTP call
- **AND** the budget count does not change

#### Scenario: Requests after the cooldown

- **WHEN** a request follows more than 5 minutes after a 429
- **THEN** the client sends it to CloudZero

Tested by: TestRateLimitCooldownSkipsRequests
