# Tasks

## 1. Report the registry name

- [x] 1.1 Change `pluginName` in `internal/plugin/plugin.go` from
      `finfocus-plugin-cloudzero` to `cloudzero`, and make
      `TestInfoDeclaresActualAndBatchOnly` assert the literal `cloudzero` for both
      `Name()` and the `GetPluginInfo` name. Verify with
      `go test ./internal/plugin/ -run TestInfoDeclaresActualAndBatchOnly`
- [x] 1.2 Confirm the binary name, module path, `.goreleaser.yaml` asset prefix,
      `plugin.manifest.json` `executable`, and the `User-Agent` in
      `internal/czclient/client.go` are unchanged. Verify with `git diff` showing no
      edits there, and `go test ./internal/goreleaser/ ./internal/czclient/` passing
- [x] 1.3 Update CLAUDE.md "Required Methods" (`Name()` returns `cloudzero`) and
      replace the "omit `--adapter`" notes in README.md and CLAUDE.md with
      `--adapter cloudzero`. Verify with markdownlint and Vale on the changed lines

## 2. Integration check

- [x] 2.1 Run `make validate` (tests, coverage gate, lint, govulncheck) and
      `openspec validate rename-plugin-name-to-cloudzero --strict`. Verify both pass
- [x] 2.2 Install a local build into a fresh `FINFOCUS_HOME` as
      `plugins/cloudzero/<version>/` and run `finfocus cost actual` on a past
      `initial-30` `dev` state with `--adapter cloudzero --cache-ttl 0` under
      `esc run finfocus-dev/cloudzero`. Verify the log shows `GetActualCost` calls
      and the results match the run without `--adapter` (8 resources, $132.72).
      About three CloudZero calls; needs the owner's go-ahead. Done 2026-11-18
      on top of #19: 8 resources, $132.72, 50 `GetActualCost` calls, no 429

## Workflow follow-up

- Archive the change in the same commit as the code, so `openspec/specs/plugin-info`
  carries the new name.
- Close #17 through the PR.
