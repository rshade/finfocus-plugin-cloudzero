# release-manifest Specification

## Purpose

Keep release artifacts installable by the FinFocus registry.

## Requirements

### Requirement: Release assets match the registry naming

GoReleaser SHALL name archives `finfocus-plugin-cloudzero_<version>_<Os>_<arch>`
with `tar.gz`, and `zip` on Windows, plus `checksums.txt`. It SHALL build no
package formats, and no workflow SHALL push a Docker image.

#### Scenario: Asset names

- **WHEN** the GoReleaser config is read
- **THEN** the archive name template matches the registry asset prefix with no version prefix

Tested by: TestAssetNamingAgainstGoreleaser, TestAssetNamingTemplateStructure, TestGoreleaserHasNoPackageFormats, TestWorkflowsDoNotPushDocker

### Requirement: Release Please tags plain semver

Release Please SHALL tag `vX.Y.Z` without a component prefix, bump patch for
features before 1.0.0, and keep a semver manifest.

#### Scenario: Tag format

- **WHEN** the release-please config is read
- **THEN** `include-component-in-tag` is false

Tested by: TestReleasePleaseTagOmitsComponent, TestReleasePleaseBumpsPatchBeforeOneZero, TestReleasePleaseManifestIsSemver
