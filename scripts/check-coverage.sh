#!/usr/bin/env bash
# Fails when statement coverage drops below the v0.1.0 targets in plan.md
# (CZ-15): 80% overall, 95% on the CloudZero client and the cost RPCs.
set -euo pipefail

module="$(go list -m)"
profile="$(mktemp)"
trap 'rm -f "$profile"' EXIT

go test -coverprofile="$profile" ./... >/dev/null

awk -v module="$module" '
  NR == 1 { next }
  {
    split($1, loc, ":")
    pkg = loc[1]
    sub("/[^/]*$", "", pkg)
    sub("^" module "/", "", pkg)
    total[pkg] += $2
    all += $2
    if ($3 > 0) {
      hit[pkg] += $2
      covered += $2
    }
  }
  function check(name, got, min) {
    printf "%-20s %6.1f%% (minimum %d%%)\n", name, got, min
    if (got < min) failed = 1
  }
  END {
    check("overall", 100 * covered / all, 80)
    check("internal/czclient", 100 * hit["internal/czclient"] / total["internal/czclient"], 95)
    check("internal/plugin", 100 * hit["internal/plugin"] / total["internal/plugin"], 95)
    exit failed
  }
' "$profile"
