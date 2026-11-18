# config-loading Specification

## Purpose

Read the plugin settings from the environment, with documented defaults.

## Requirements

### Requirement: Settings come from CLOUDZERO_ variables with defaults

The plugin SHALL read `CLOUDZERO_API_KEY`, `CLOUDZERO_BASE_URL` (default
`https://api.cloudzero.com/v2`), `CLOUDZERO_COST_TYPE` (default `real_cost`),
`CLOUDZERO_CURRENCY` (default `USD`), `CLOUDZERO_RESOURCE_DIMENSION`,
`CLOUDZERO_TAG_DIMENSIONS`, `CLOUDZERO_DAILY_BUDGET` (default 50),
`CLOUDZERO_CACHE_TTL` (default 6h), and `CLOUDZERO_RECENT_TTL` (default 1h).
It SHALL start without a key.

#### Scenario: Empty environment

- **WHEN** no variable is set
- **THEN** every default applies and the plugin reports no key

Tested by: TestLoadDefaults, TestLoadEveryVariable

### Requirement: Invalid values fail loading

A cost type outside the eight documented values, a budget that is not a
positive integer, a TTL that is not a positive duration, and a tag dimension
entry that is not `tagKey=dimensionID` SHALL fail loading with an error that
names the variable.

#### Scenario: Unknown cost type

- **WHEN** `CLOUDZERO_COST_TYPE` is not a documented cost type
- **THEN** loading fails

Tested by: TestLoadRejectsInvalidValues, TestLoadAcceptsDocumentedCostTypes, TestParseTagDimensionsRejectsEmptySide
