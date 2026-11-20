# Tasks

## 1. Read recommendations from CloudZero

- [x] 1.1 Move `live-capture-2026-11-19.json` from this change folder to
      `internal/czclient/testdata/live/recommendations.json`. It is the
      2026-11-19 response (five of eight rows, `limit=5`), already sanitized:
      account `123456789012`, made-up volume ids and recommendation hashes,
      and a placeholder cursor. Verify that the plugin tests read it
- [x] 1.2 Add `czclient.Recommendation` (id, title, type id and name, effort,
      status, resource, resource type and name, cost impact, created,
      accounts, regions, cloud providers) and `parseRecommendations`, which
      drops rows without an id or resource and treats null lists as empty.
      Verify with `TestParseRecommendations` (the fixture, a null resource, a
      non-JSON body)
- [x] 1.3 Add `Client.GetRecommendations`: `GET /optimize/recommendations`
      with `cost_impact_last_30_days[gte]=0.01` and `status` for
      `not_started`, `in_progress`, and `ignored`, cursor paging, first page
      charged through `admit`, one restart on 410. Verify with
      `TestGetRecommendationsQueryAndPages` and
      `TestGetRecommendationsRespectsCooldown`

## 2. Resolve identity without the resource dimension

- [x] 2.1 Split `Mapper.Identity` out of `Mapper.Filter`. `Filter` keeps the
      dimension check and the shared filters. Verify with
      `TestIdentityWithoutResourceDimension` and the existing mapping tests
- [x] 2.2 Read the descriptor's `pulumi:arn` and `arn` tags in
      `identifiersOf`, after the descriptor `arn` and before `id`. Verify with
      a new case in `TestFilterIdentityPrecedence` and the existing cost tests
      unchanged

## 3. Serve GetRecommendations

- [x] 3.1 Add `RecommendationSource` to the `CloudZero` interface and the
      test fake. Verify with `make build` and `make test`
- [x] 3.2 Add `internal/plugin/recommendations.go`: `GetRecommendations`
      checks the key (`FailedPrecondition`), reads the cached list through
      `inflight` with `CLOUDZERO_RECENT_TTL`, scopes it to the targets with
      `Mapper.Identity` and `Filter.Matches` (target id as resource id, CZRN
      without targets), maps each row, and applies the SDK visibility,
      filter, paging, and summary helpers in that order. Errors go through
      `statusFrom` and are not cached. Verify with
      `TestGetRecommendationsScopesToTargets`,
      `TestGetRecommendationsWithoutTargets`,
      `TestGetRecommendationsMapsFields`,
      `TestGetRecommendationsScalesSavings`,
      `TestGetRecommendationsVisibility`,
      `TestGetRecommendationsPagesAndFilters`,
      `TestGetRecommendationsCachesOneFetch`, and
      `TestGetRecommendationsErrors`
- [x] 3.3 Confirm `DismissRecommendation` and `GetRecommendationHistory`
      return `Unimplemented` through the SDK server. Verify with
      `TestRecommendationWritesUnimplemented`
- [x] 3.4 Add `RECOMMENDATIONS` to `Info` and `Supports`. Extend
      `TestInfoDeclaresCapabilities`, `TestSupports`, and
      `TestBinaryAnnouncesPortAndCapabilities`. Verify with `make test`,
      which includes `test/conformance`

## 4. Document

- [x] 4.1 Update `CONTEXT.md` (recommendations in Data Source of Truth),
      `README.md` (recommendations, the one-cent floor, dismissal kept by
      core), `docs/design.md`, and `CLAUDE.md` (Protocol and RPC Methods).
      Verify with markdownlint

## 5. Validate

- [x] 5.1 Run `make validate` and
      `mise exec -- openspec validate add-cloudzero-recommendations --strict`
- [x] 5.2 With the owner's go-ahead, run `finfocus cost recommendations`
      against an exported `initial-30` `dev` state with only this plugin
      installed, and check that CloudZero accepts the query. Done 2026-11-19:
      the repeated `status` values were accepted (no error, one request), and
      31 targets got 0 recommendations. Every recommended volume is an
      instance's root volume, which is not a stack resource, and they belong
      to the instances of stack version 131, replaced on 2026-11-18.
      Attaching a root-volume recommendation to its instance is
      `attach-root-volume-recommendations` (#29), which needs core to send
      `attributes` on recommendation targets (rshade/finfocus#2281)

## Workflow follow-up

- Archive `add-cloudzero-budgets` first. This change's `plugin-info` delta
  modifies a requirement that change creates.
- Archive this change in the same commit as the code.
