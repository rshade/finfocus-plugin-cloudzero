# Spec Delta

## ADDED Requirements

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
