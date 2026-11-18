# plugin-info Specification

## Purpose

Identify the plugin to FinFocus core and declare only what it can answer.

## Requirements

### Requirement: Declared capabilities are actual cost and batch cost

`GetPluginInfo` SHALL report the name `finfocus-plugin-cloudzero`, the providers
`aws`, `azure`, and `gcp`, and exactly the capabilities `ACTUAL_COSTS` and
`BATCH_COST`. The SDK server SHALL serve the same list.

#### Scenario: Plugin info lists two capabilities

- **WHEN** core calls `GetPluginInfo`
- **THEN** the capabilities are `ACTUAL_COSTS` and `BATCH_COST`, in that order

#### Scenario: The built binary serves the declared capabilities

- **WHEN** the binary starts and prints `PORT=<n>` on stdout
- **THEN** `GetPluginInfo` over gRPC on that port returns the declared capabilities

Tested by: TestInfoDeclaresActualAndBatchOnly, TestBinaryAnnouncesPortAndCapabilities

### Requirement: Supports declines anything the plugin cannot answer

`Supports` SHALL decline, with a non-empty reason, a missing descriptor, a
provider other than aws, azure, or gcp, any capability other than
`ACTUAL_COSTS` or `BATCH_COST` (including an unspecified one), a missing API
key, and a missing resource dimension. Otherwise it SHALL answer supported.

#### Scenario: Ready plugin supports actual cost for AWS

- **WHEN** the key and resource dimension are set and the request is AWS actual cost
- **THEN** `Supports` answers supported

#### Scenario: Unspecified capability is declined

- **WHEN** the request names no capability
- **THEN** `Supports` declines with a reason

Tested by: TestSupports

### Requirement: Projected cost, pricing spec, and estimate are unimplemented

`GetProjectedCost`, `GetPricingSpec`, and `EstimateCost` SHALL return
`Unimplemented`. They SHALL NOT return a `$0` result, because core treats it as
a real price.

#### Scenario: Projected cost is unimplemented

- **WHEN** core calls `GetProjectedCost`
- **THEN** the status code is `Unimplemented`

Tested by: TestUnimplementedCostRPCs, TestSDKConformance
