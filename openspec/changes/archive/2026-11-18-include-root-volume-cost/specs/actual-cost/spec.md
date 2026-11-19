# Spec Delta

## ADDED Requirements

### Requirement: Root volume cost is reported as its own source

Rows for a resource's root volume SHALL be returned as separate daily results
whose source is the resource's source followed by "root volume", so the total
includes them and core's breakdown shows them on their own line. They SHALL
come from the same month query, with no extra CloudZero call, in both
`GetActualCost` and `BatchCost`.

#### Scenario: An instance and its root volume

- **WHEN** an instance bills $10 and its root volume $0.80 in the window
- **THEN** the results total $10.80, and $0.80 of it carries the root volume source

#### Scenario: Another instance's volume

- **WHEN** the month query also returns a volume that belongs to a different instance
- **THEN** it is not added to this instance

Tested by: TestGetActualCostIncludesRootVolume, TestBatchCostAgainstLiveFixture
