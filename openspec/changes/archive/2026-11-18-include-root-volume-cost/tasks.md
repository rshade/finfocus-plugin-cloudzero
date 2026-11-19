# Tasks

## 1. Match the root volume

- [x] 1.1 Add `RootVolumeSuffix` to `mapping.Filter` and `Filter.MatchesRootVolume`,
      and set the suffix in `Mapper.Filter` from `attributes.rootBlockDevice.volumeId`
      (object or first list entry, AWS only, `vol-` ids only). Verify with
      `TestFilterRootVolume` (object, list, data volume only, no attribute,
      non-AWS provider, CloudProvider mismatch)

## 2. Report it

- [x] 2.1 In `rowsFor` (`internal/plugin/actual.go`), emit rows that match the root
      volume as separate results with "root volume" appended to the source.
      Verify with `TestGetActualCostIncludesRootVolume` (sum, source split, another
      instance's volume left out, one CloudZero call)
- [x] 2.2 Extend `TestBatchCostAgainstLiveFixture`: give the Windows instance the
      fixture's $2.99 volume and a Linux instance a $0.80 one, and check the totals
      and that the query count is unchanged
- [x] 2.3 Update README.md and `docs/design.md` (remove the root-volume limitation)
      and CLAUDE.md. Verify with markdownlint

## 3. Validate

- [x] 3.1 Run `make validate` and `openspec validate include-root-volume-cost --strict`
