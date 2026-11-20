# Tasks

## 1. Read budgets from CloudZero

- [x] 1.1 Add `czclient.Budget` (id, name, view id, monthly planned limits
      keyed by month, enabled alert percentages, current planned and actual,
      each amount optional) and `parseBudgets`, accepting numeric strings and
      numbers and `YYYY-MM-DD` or `YYYY-MM` keys. Add a docs-shaped fixture
      `internal/czclient/testdata/budgets_docs.json` with no real ids. Verify
      with `TestParseBudgets` (string and number amounts, both key forms, an
      unparsable amount, a non-JSON body)
- [x] 1.2 Add `Client.GetBudgets`: `GET /budgets?expand=current`, cursor paging
      as in `pages`, first page charged through `admit`, one restart on 410.
      Verify with `TestGetBudgetsPagesAndCharges` (two pages, one budget unit
      spent, `expand=current` sent) and `TestGetBudgetsRespectsCooldown`
- [x] 1.3 Make the bad-request and rate-limit texts in `czclient/errors.go`
      name the CloudZero request without saying "cost", for example
      "CloudZero rejected the request" and "CloudZero allows 60 requests in a
      rolling 24 hours". Verify with `TestAPIErrorText` and `TestLiveErrorBodies`

## 2. Serve GetBudgets

- [x] 2.1 Make `inflight` generic over its result type and keep the cost path
      on it. Verify with the existing `inflight_test.go` and `make test`
- [x] 2.2 Add the `BudgetSource` and `CloudZero` interfaces and change
      `plugin.New` to take `CloudZero`. Extend the test fake with
      `GetBudgets`. Verify with `make build` and `make test`
- [x] 2.3 Add `internal/plugin/budgets.go`: `GetBudgets` checks the key
      (`FailedPrecondition`), reads through the cache and `inflight` with
      `CLOUDZERO_RECENT_TTL`, drops budgets that are not monthly or have no
      planned amount this month, and maps each budget (current UTC month limit,
      monthly period, configured currency, source `cloudzero`, view id in
      metadata, actual thresholds in ascending order, spend-only status) and
      the summary total. Errors go through `statusFrom` and are not cached.
      Verify with `TestGetBudgetsMapsCloudZeroBudgets`,
      `TestGetBudgetsSkipsUnplannedMonth`, `TestGetBudgetsStatusIsSpendOnly`,
      `TestGetBudgetsThresholds`, `TestGetBudgetsSummary`,
      `TestGetBudgetsCachesOneFetch`, and `TestGetBudgetsErrors`
- [x] 2.4 Add `BUDGETS` to `Info` and to `Supports`, which asks for the
      resource dimension only for `ACTUAL_COSTS` and `BATCH_COST`. Rename
      `TestInfoDeclaresActualAndBatchOnly` to `TestInfoDeclaresCapabilities`
      and extend `TestSupports` and `TestBinaryAnnouncesPortAndCapabilities`.
      Verify with `make test`, which includes `test/conformance`

## 3. Document

- [x] 3.1 Update `CONTEXT.md` (identity: a read-only plugin for what CloudZero
      has billed and recorded; budgets in Data Source of Truth), `README.md`
      (budgets, with no new settings), `docs/design.md`, and `CLAUDE.md`
      (Protocol and RPC Methods). Verify with markdownlint

## 4. Validate

- [x] 4.1 Run `make validate` and
      `mise exec -- openspec validate add-cloudzero-budgets --strict`

## Workflow follow-up

- An owner with a CloudZero budget runs one live `GET /v2/budgets?expand=current`
  and replaces the docs-shaped fixture with a sanitized capture.
- Archive the change in the same commit as the code.
