# Spec Delta

## ADDED Requirements

### Requirement: A cooldown error names what is left of the stated wait

When the 429 that started a cooldown stated a wait, every rate-limit error the
cooldown returns SHALL carry the part of that wait still to run, rounded up to
whole minutes in its text. A 429 that stated no wait SHALL leave cooldown
errors saying "retry later".

#### Scenario: A retry inside the cooldown

- **WHEN** CloudZero answered 429 saying to wait 24 minutes, and a request follows 90 seconds later
- **THEN** the request is not sent and its error says "retry in 23 minutes"

Tested by: TestRateLimitCooldownKeepsStatedWait, TestAPIErrorText

## MODIFIED Requirements

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
