# Proposal

## Why

`--adapter` cannot select this plugin. FinFocus core checks the flag against two
names: registry discovery matches the install name `cloudzero`, and the engine
matches the plugin's `Name()` RPC, which reports `finfocus-plugin-cloudzero`.
Spike #8 measured it on 2026-11-18: neither value makes a single
`GetActualCost` call, and the command returns an empty result with no warning.
Reporting `cloudzero` makes the two names agree, and matches vantage, opencost,
and flexera, which report their registry names. This is issue #17.

## What Changes

- The `Name()` RPC and the `GetPluginInfo` name change from
  `finfocus-plugin-cloudzero` to `cloudzero`. **BREAKING** for anything that
  reads the reported name: the `adapter` field of FinFocus results changes to
  `cloudzero`.
- `--adapter cloudzero` selects the plugin when it is installed from the
  registry.
- Unchanged: the binary name, module path, GoReleaser asset prefix,
  `plugin.manifest.json` `executable`, the HTTP `User-Agent`
  (`finfocus-plugin-cloudzero/<version>`), and the registry entry.
- README and CLAUDE.md drop the advice to omit `--adapter`.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `plugin-info`: the requirement "Declared capabilities are actual cost and
  batch cost" names the reported plugin name, which becomes `cloudzero`, and it
  gains a scenario for `--adapter` selection.

## Impact

- Code: `internal/plugin/plugin.go` (`pluginName`), and
  `internal/plugin/plugin_test.go`, which compares against the constant rather
  than the literal name.
- Users: scripts or dashboards that match the result `adapter` field on
  `finfocus-plugin-cloudzero` must match `cloudzero`.
- Core: no change needed for this plugin. rshade/finfocus#2206 separately lets
  `--adapter` match either name for plugins that keep a different `Name()`.
