package goreleaser_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestAssetNamingAgainstGoreleaser validates that the .goreleaser.yaml template
// produces asset names matching the finfocus installer's patterns.
//
// This test:
//  1. Reads the actual .goreleaser.yaml file
//  2. Extracts the archive name_template and format_overrides
//  3. Renders the template using text/template with custom functions
//  4. Appends the correct extension (tar.gz for Unix, zip for Windows)
//  5. Validates each rendered name against installer patterns from
//     ../finfocus/internal/registry/github.go buildAssetPatterns
//
// The finfocus installer looks for patterns like:
//
//	finfocus-plugin-cloudzero_v0.1.0_Linux_x86_64.tar.gz
//	finfocus-plugin-cloudzero_v0.1.0_Darwin_arm64.tar.gz
//	finfocus-plugin-cloudzero_v0.1.0_Windows_x86_64.zip
//
// With variations (based on buildAssetPatterns logic):
// - OS: goos, Title-case (Linux, Darwin, Windows), darwin aliases (macos, macOS, MacOS).
// - Arch: goarch (amd64, arm64), x86_64/X86_64/AMD64 (for amd64), ARM64/aarch64/AARCH64 (for arm64).
// - Version: with or without leading v.
func TestAssetNamingAgainstGoreleaser(t *testing.T) {
	t.Parallel()

	// Read the goreleaser config
	configContent, err := os.ReadFile("../../.goreleaser.yaml")
	require.NoError(t, err, "failed to read .goreleaser.yaml")

	var config struct {
		ProjectName string `yaml:"project_name"`
		Archives    []struct {
			NameTemplate    string   `yaml:"name_template"`
			Formats         []string `yaml:"formats"`
			FormatOverrides []struct {
				Goos    string   `yaml:"goos"`
				Formats []string `yaml:"formats"`
			} `yaml:"format_overrides"`
		} `yaml:"archives"`
	}
	err = yaml.Unmarshal(configContent, &config)
	require.NoError(t, err, "failed to parse .goreleaser.yaml")

	require.NotEmpty(t, config.Archives, "no archives defined in .goreleaser.yaml")
	require.NotEmpty(t, config.ProjectName, "project_name is empty in .goreleaser.yaml")

	archive := config.Archives[0]
	require.NotEmpty(t, archive.NameTemplate, "archive name_template is empty")

	// Parse the name_template with text/template
	tmpl, err := template.New("name").Funcs(template.FuncMap{
		"title": func(s string) string {
			if len(s) == 0 {
				return s
			}
			return strings.ToUpper(s[:1]) + s[1:]
		},
	}).Parse(archive.NameTemplate)
	require.NoError(t, err, "failed to parse name_template")

	// buildInstallerPatterns generates all valid asset names the installer would accept.
	// Based on buildAssetPatterns from ../finfocus/internal/registry/github.go
	buildInstallerPatterns := func(projectName, version, goos, goarch string) []string {
		var patterns []string

		// OS names: goos, Title-case, and darwin aliases
		osNames := []string{goos}
		titleOS := strings.ToUpper(goos[:1]) + goos[1:]
		osNames = append(osNames, titleOS)
		if goos == "darwin" {
			osNames = append(osNames, "macos", "macOS", "MacOS")
		}

		// Arch names: goarch, plus amd64/arm64 aliases
		archNames := []string{goarch}
		if goarch == "amd64" {
			archNames = append(archNames, "x86_64", "X86_64", "AMD64")
		}
		if goarch == "arm64" {
			archNames = append(archNames, "ARM64", "aarch64", "AARCH64")
		}

		// Version variations: with and without leading v
		versions := []string{version}
		if ver, ok := strings.CutPrefix(version, "v"); ok {
			versions = append(versions, ver)
		} else {
			versions = append(versions, "v"+version)
		}

		// Generate all valid pattern prefixes (extension added later)
		for _, ver := range versions {
			for _, osName := range osNames {
				for _, arch := range archNames {
					pattern := projectName + "_" + ver + "_" + osName + "_" + arch
					patterns = append(patterns, pattern)
				}
			}
		}

		return patterns
	}

	// Test cases: all OS/arch combinations with versions
	testCases := []struct {
		name    string
		goos    string
		goarch  string
		version string
	}{
		// Linux combinations
		{
			name:    "linux amd64 with v prefix",
			goos:    "linux",
			goarch:  "amd64",
			version: "v0.1.0",
		},
		{
			name:    "linux arm64 with v prefix",
			goos:    "linux",
			goarch:  "arm64",
			version: "v0.1.0",
		},
		{
			name:    "linux amd64 without v prefix",
			goos:    "linux",
			goarch:  "amd64",
			version: "0.1.0",
		},
		// Darwin (macOS) combinations
		{
			name:    "darwin amd64 (macOS x86_64) with v prefix",
			goos:    "darwin",
			goarch:  "amd64",
			version: "v0.1.0",
		},
		{
			name:    "darwin arm64 (macOS ARM) with v prefix",
			goos:    "darwin",
			goarch:  "arm64",
			version: "v0.1.0",
		},
		// Windows combinations
		{
			name:    "windows amd64 with v prefix",
			goos:    "windows",
			goarch:  "amd64",
			version: "v0.1.0",
		},
		{
			name:    "windows arm64 with v prefix",
			goos:    "windows",
			goarch:  "arm64",
			version: "v0.1.0",
		},
		{
			name:    "windows amd64 without v prefix",
			goos:    "windows",
			goarch:  "amd64",
			version: "0.1.0",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			ext := archiveExtension(archive.Formats)
			for _, override := range archive.FormatOverrides {
				if override.Goos == tc.goos {
					ext = archiveExtension(override.Formats)
					break
				}
			}

			// Render the template with actual goreleaser context
			var rendered strings.Builder
			err := tmpl.Execute(&rendered, map[string]any{
				"ProjectName": config.ProjectName,
				"Version":     tc.version,
				"Os":          tc.goos,
				"Arch":        tc.goarch,
			})
			require.NoError(t, err, "failed to render template for %s/%s", tc.goos, tc.goarch)

			assetName := rendered.String() + ext

			// Generate valid patterns the installer would look for
			patterns := buildInstallerPatterns(config.ProjectName, tc.version, tc.goos, tc.goarch)

			// Build full asset names with extensions
			var validAssets []string
			for _, pattern := range patterns {
				validAssets = append(validAssets, pattern+ext)
			}

			// Verify the rendered name matches at least one valid pattern
			matched := slices.Contains(validAssets, assetName)

			assert.True(t, matched,
				"rendered asset name %q not found in installer patterns for goos=%s, goarch=%s, version=%s",
				assetName, tc.goos, tc.goarch, tc.version)
		})
	}
}

// TestAssetNamingTemplateStructure validates that the goreleaser template
// uses correct separators and function calls to avoid common mistakes.
func TestAssetNamingTemplateStructure(t *testing.T) {
	t.Parallel()

	configContent, err := os.ReadFile("../../.goreleaser.yaml")
	require.NoError(t, err)

	var config struct {
		Archives []struct {
			NameTemplate string `yaml:"name_template"`
		} `yaml:"archives"`
	}
	err = yaml.Unmarshal(configContent, &config)
	require.NoError(t, err)

	require.NotEmpty(t, config.Archives)
	tmpl := config.Archives[0].NameTemplate

	// Verify critical template structure
	assert.Contains(t, tmpl, "{{ .ProjectName }}", "template must include ProjectName")
	assert.Contains(t, tmpl, "{{ .Version }}", "template must include Version")
	assert.Contains(t, tmpl, ".Os", "template must reference Os")
	assert.Contains(t, tmpl, "{{ .Arch }}", "template must include Arch")

	// Verify it uses underscores as separators (not dashes)
	assert.Contains(t, tmpl, "_{{", "template must use underscores as separators")
	assert.NotContains(t, tmpl, "-{{ .Os }}", "template must not use dashes before Os")
	assert.NotContains(t, tmpl, "-{{ .Arch }}", "template must not use dashes before Arch")

	// Verify correct OS capitalization is handled (title filter or function)
	assert.Contains(t, tmpl, "title", "template must use title filter for OS capitalization")

	// Verify correct arch transformation for amd64
	assert.Contains(t, tmpl, "x86_64", "template must map amd64 to x86_64 for installer compatibility")

	// Verify no common mistakes: wrong OS names, wrong separators
	assert.NotContains(t, tmpl, "Macos", "template must not use 'Macos' (should use title filter)")
}

func archiveExtension(formats []string) string {
	if len(formats) == 0 {
		return ".tar.gz"
	}
	return "." + formats[0]
}

func TestReleasePleaseTagOmitsComponent(t *testing.T) {
	t.Parallel()
	body := readRepoFile(t, "release-please-config.json")
	var cfg struct {
		Packages map[string]struct {
			IncludeComponentInTag *bool `json:"include-component-in-tag"`
		} `json:"packages"`
	}
	require.NoError(t, json.Unmarshal(body, &cfg))
	root, ok := cfg.Packages["."]
	require.True(t, ok, "release-please-config.json has no root package")
	require.NotNil(t, root.IncludeComponentInTag)
	assert.False(t, *root.IncludeComponentInTag)
}

func TestReleasePleaseBumpsPatchBeforeOneZero(t *testing.T) {
	t.Parallel()
	body := readRepoFile(t, "release-please-config.json")
	var cfg struct {
		Packages map[string]struct {
			BumpMinorPreMajor         *bool  `json:"bump-minor-pre-major"`
			BumpPatchForMinorPreMajor *bool  `json:"bump-patch-for-minor-pre-major"`
			InitialVersion            string `json:"initial-version"`
		} `json:"packages"`
	}
	require.NoError(t, json.Unmarshal(body, &cfg))
	root, ok := cfg.Packages["."]
	require.True(t, ok, "release-please-config.json has no root package")
	require.NotNil(t, root.BumpMinorPreMajor)
	assert.True(t, *root.BumpMinorPreMajor)
	require.NotNil(t, root.BumpPatchForMinorPreMajor)
	assert.True(t, *root.BumpPatchForMinorPreMajor)
	assert.Equal(t, "0.1.0", root.InitialVersion)
}

func TestReleasePleaseManifestIsSemver(t *testing.T) {
	t.Parallel()
	body := readRepoFile(t, ".release-please-manifest.json")
	var manifest map[string]string
	require.NoError(t, json.Unmarshal(body, &manifest))
	version, ok := manifest["."]
	require.True(t, ok)
	assert.Regexp(t, `^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$`, version)
}

func TestGoreleaserHasNoPackageFormats(t *testing.T) {
	t.Parallel()
	body := string(readRepoFile(t, ".goreleaser.yaml"))
	for _, section := range []string{"\ndocker:", "\nbrews:", "\nnfpms:"} {
		assert.NotContains(t, body, section)
	}
	assert.Contains(t, body, "formats:")
	assert.NotRegexp(t, `(?m)^format:`, body)
}

func TestWorkflowsDoNotPushDocker(t *testing.T) {
	t.Parallel()
	matches, err := filepath.Glob("../../.github/workflows/*")
	require.NoError(t, err)
	require.NotEmpty(t, matches)
	for _, path := range matches {
		body, err := os.ReadFile(path)
		require.NoError(t, err)
		assert.NotContains(t, string(body), "docker/build-push-action", path)
	}
}

func readRepoFile(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("../..", name))
	require.NoError(t, err)
	return body
}
