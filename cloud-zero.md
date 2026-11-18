# CloudZero Plugin Planning

**Note:** This document has been superseded by [`docs/design.md`](docs/design.md), which
contains the current architectural design and implementation guidance. This file is
retained for historical reference.

## Prerequisite Tasks

The following infrastructure improvements are required before starting the CloudZero
plugin to ensure optimal performance and adherence to API rate limits:

1. **Spec Update:** Add `expires_at` to `ActualCostResult` and `ProjectedCostResult` ([finfocus-spec#380](https://github.com/rshade/finfocus-spec/issues/380)).
2. **CLI Upgrade:** Transition persistent cache from JSON to BoltDB (bbolt)
   ([finfocus#674](https://github.com/rshade/finfocus/issues/674)).
    * *Context:* Current `engine/cache` implementation using `FileStore` (JSON-per-file)
      is experiencing performance issues and goroutine fan-out under load
      ([finfocus#653](https://github.com/rshade/finfocus/issues/653)). Replacing this
      with BoltDB will provide a more robust, atomic, and performant storage layer.

3. **Batch Processing:** Implement dedicated batch RPCs for efficient multi-resource
   cost queries ([finfocus-spec#221](https://github.com/rshade/finfocus-spec/issues/221)).

## Architecture & Integration Strategy

... (rest of the content remains the same)

* **Plugin Strategy:** The plugin should be optimized for receiving a list of 100+ resources at once.
* **CLI Interaction:** By prioritizing Batch RPCs, we ensure the CLI bundles resource
  requests, allowing the plugin to perform a single "Dimension Query" against CloudZero
  to satisfy the entire batch.
* **Sequential Fallback:** If `GetActualCost` (single resource) is called, the plugin
  will check its local BoltDB cache first. If it's a cache miss, it will trigger a
  background sync for the relevant environment/account to avoid burning API credits on
  one-off lookups.

### 2. Dimension Mapping: The Plugin's Responsibility

The CLI must remain provider-agnostic. It sends raw ARNs, URNs, and Tags. It is the
**Plugin's job** to translate these into CloudZero Dimensions:

* **ARN to Account/Service:** Parse AWS/GCP ARNs to extract Account IDs and Service
  names (e.g., `lambda`, `ec2`).
* **Tags to Custom Dimensions:** Map a configured tag key to a dimension ID.
  Example: `Team=User:Defined:Team`. Do not invent the ID from the tag name.
* **Multi-Cloud Normalization:** The plugin handles the logic of knowing that a Google
  Cloud URN and an AWS ARN both map to a unified "Compute" service dimension in
  CloudZero.

### 3. The Caching Dilemma: Plugin vs. CLI

The CloudZero API's limit dictates a multi-tier cache.
... (rest of the content remains the same)

---

## Estimated Tasks

1. **Spec Update:** Add `expires_at` to `ActualCostResponse` in `finfocus-spec`.
2. **CLI Upgrade:** Refactor `finfocus-cli` storage layer to use `etcd-io/bbolt`.
3. **Plugin Development:** Build `finfocus-plugin-cloudzero` with a background sync worker.
