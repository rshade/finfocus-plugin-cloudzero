# Spec Delta

## MODIFIED Requirements

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
