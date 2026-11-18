# Spec Delta

## MODIFIED Requirements

### Requirement: Declared capabilities are actual cost and batch cost

`GetPluginInfo` and the `Name` RPC SHALL report the name `cloudzero`, which is
the plugin's FinFocus registry name. `GetPluginInfo` SHALL report the providers
`aws`, `azure`, and `gcp`, and exactly the capabilities `ACTUAL_COSTS` and
`BATCH_COST`. The SDK server SHALL serve the same list.

#### Scenario: Plugin info lists two capabilities

- **WHEN** core calls `GetPluginInfo`
- **THEN** the capabilities are `ACTUAL_COSTS` and `BATCH_COST`, in that order

#### Scenario: Reported name is the registry name

- **WHEN** core calls `Name` or `GetPluginInfo`
- **THEN** the name is `cloudzero`, so `--adapter cloudzero` matches both the
  registry install name and the reported name

#### Scenario: The built binary serves the declared capabilities

- **WHEN** the binary starts and prints `PORT=<n>` on stdout
- **THEN** `GetPluginInfo` over gRPC on that port returns the declared capabilities

Tested by: TestInfoDeclaresActualAndBatchOnly, TestBinaryAnnouncesPortAndCapabilities
