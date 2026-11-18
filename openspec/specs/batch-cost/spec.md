# batch-cost Specification

## Purpose

Answer actual cost for many resources with one CloudZero query per shared
filter set.

## Requirements

### Requirement: One query per shared filter set, results in request order

`BatchCost` with the actual query type SHALL group resources by their shared
filters and query CloudZero once per group. Results SHALL keep request order,
one per resource, duplicates included. A resource that cannot be mapped SHALL
get its own `InvalidArgument` error without failing the others.

#### Scenario: Two instances, a duplicate, and an unmappable resource

- **WHEN** a batch holds two instances in one account, a repeat of the first, and a resource with no id
- **THEN** CloudZero is called once
- **AND** the first three results carry their costs and the fourth is `InvalidArgument`

#### Scenario: A live-shaped response prices a real stack

- **WHEN** the captured CloudZero response serves eight resources, two of them NAT gateways with no ARN
- **THEN** every resource gets its captured cost from two CloudZero calls

Tested by: TestBatchCostOneCallKeepsOrderAndPartialFailure, TestBatchCostAgainstLiveFixture

### Requirement: Failures and unsupported query types are per resource

An upstream failure SHALL fail only the resources in that group, mapped to a
gRPC code. A missing key or resource dimension SHALL fail every resource with
`FailedPrecondition` without calling CloudZero. A non-actual query type SHALL
fail every resource with `Unimplemented`. An empty or nil request SHALL return
no results. An empty window SHALL be an error.

#### Scenario: Projected batch

- **WHEN** the query type is projected
- **THEN** each result is `Unimplemented`

#### Scenario: Plugin not ready

- **WHEN** no API key is configured
- **THEN** each result is `FailedPrecondition` and CloudZero is not called

Tested by: TestBatchCostUpstreamFailure, TestBatchCostNotReady, TestBatchCostProjectedEmptyAndBadWindow
