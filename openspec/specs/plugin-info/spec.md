# plugin-info Specification

## Purpose

Identify the plugin to FinFocus core and declare only what it can answer.

## Requirements

### Requirement: Declared capabilities are what CloudZero can answer

`GetPluginInfo` and the `Name` RPC SHALL report the name `cloudzero`, which is
the plugin's FinFocus registry name. `GetPluginInfo` SHALL report the providers
`aws`, `azure`, and `gcp`, and exactly the capabilities `ACTUAL_COSTS`,
`BATCH_COST`, and `BUDGETS`. The SDK server SHALL serve the same list.

#### Scenario: Plugin info lists every declared capability

- **WHEN** core calls `GetPluginInfo`
- **THEN** the capabilities are `ACTUAL_COSTS`, `BATCH_COST`, and `BUDGETS`, in that order

#### Scenario: Reported name is the registry name

- **WHEN** core calls `Name` or `GetPluginInfo`
- **THEN** the name is `cloudzero`, so `--adapter cloudzero` matches both the
  registry install name and the reported name

#### Scenario: The built binary serves the declared capabilities

- **WHEN** the binary starts and prints `PORT=<n>` on stdout
- **THEN** `GetPluginInfo` over gRPC on that port returns the declared capabilities

Tested by: TestInfoDeclaresCapabilities, TestBinaryAnnouncesPortAndCapabilities

### Requirement: Supports declines anything the plugin cannot answer

`Supports` SHALL decline, with a non-empty reason, a missing descriptor, a
provider other than aws, azure, or gcp, any capability other than
`ACTUAL_COSTS`, `BATCH_COST`, or `BUDGETS` (including an unspecified one), and
a missing API key. It SHALL decline a missing resource dimension for
`ACTUAL_COSTS` and `BATCH_COST` only. Otherwise it SHALL answer supported.

#### Scenario: Ready plugin supports actual cost for AWS

- **WHEN** the key and resource dimension are set and the request is AWS actual cost
- **THEN** `Supports` answers supported

#### Scenario: Unspecified capability is declined

- **WHEN** the request names no capability
- **THEN** `Supports` declines with a reason

#### Scenario: Budgets need only the key

- **WHEN** the key is set, no resource dimension is set, and the request is AWS budgets
- **THEN** `Supports` answers supported

Tested by: TestSupports

### Requirement: Projected cost, pricing spec, and estimate are unimplemented

`GetProjectedCost`, `GetPricingSpec`, and `EstimateCost` SHALL return
`Unimplemented`. They SHALL NOT return a `$0` result, because core treats it as
a real price.

#### Scenario: Projected cost is unimplemented

- **WHEN** core calls `GetProjectedCost`
- **THEN** the status code is `Unimplemented`

Tested by: TestUnimplementedCostRPCs, TestSDKConformance
