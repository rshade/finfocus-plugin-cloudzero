# Tasks

## 1. Resolve the root volume in identity

- [x] 1.1 Move the `RootVolumeSuffix` lookup from `Mapper.Filter` into
      `Mapper.identity`, so `Identity` carries it. Verify with a root-volume
      case in `TestIdentityWithoutResourceDimension` and `TestFilterRootVolume`
      unchanged

## 2. Attach root-volume recommendations

- [x] 2.1 In `internal/plugin/recommendations.go`, make `matchesTarget` accept
      `MatchesRootVolume` as well as `Matches`, and add `cloudzero_resource`
      (the CZRN) to every recommendation's metadata. Verify with
      `TestGetRecommendationsAttachesRootVolume` (an object and a list
      `rootBlockDevice`, a data volume, a target without attributes) and the
      updated `TestGetRecommendationsMapsFields`

## 3. Document

- [x] 3.1 Replace the root-volume limitation in README "Recommendations" with
      the behaviour and its dependency on core (rshade/finfocus#2281), and
      update docs/design.md and CLAUDE.md. Verify with markdownlint

## 4. Validate

- [x] 4.1 Run `make validate` and
      `mise exec -- openspec validate attach-root-volume-recommendations --strict`
- [x] 4.2 With the owner's go-ahead, build core from the #2281 branch, run
      `finfocus cost recommendations` on an exported `initial-30` `dev` state
      with only this plugin installed, and check that the root-volume
      recommendations appear on their instances. Done 2026-11-19 on the stack
      version 131 export, whose instances own the recommended volumes: six
      recommendations, one per instance, $0.79 a month. The current state gets
      none, because its instances were replaced after CloudZero made them. A
      core without #2281 gets none on the same data (local fake CloudZero)

## Workflow follow-up

- Archive `add-cloudzero-recommendations` first. This change modifies the
  `recommendations` spec it creates.
- Archive this change in the same commit as the code. Close #29.
