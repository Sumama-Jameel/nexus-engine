#!/usr/bin/env bash
# coverage-ratchet.sh — Prevent coverage regression
#
# This script enforces the "coverage ratchet" principle:
#   1. Total coverage must never fall below MINIMUM_THRESHOLD
#   2. Per-package coverage must never fall below PACKAGE_MINIMUM
#   3. When coverage increases, the threshold file is updated automatically
#
# Usage:
#   ./scripts/coverage-ratchet.sh          # Check and auto-update
#   ./scripts/coverage-ratchet.sh --check   # Check only (CI mode — fail if below)
#   ./scripts/coverage-ratchet.sh --update  # Force update threshold file
#
# The threshold is stored in .coverage-threshold for persistence across runs.

set -euo pipefail

# ─── Configuration ───

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"
THRESHOLD_FILE="${PROJECT_DIR}/.coverage-threshold"

# Minimum total coverage (absolute floor — the ratchet baseline)
MINIMUM_THRESHOLD=80

# Per-package minimums (package=minimum)
declare -A PACKAGE_MINIMUMS=(
    ["internal/engine"]=80
    ["internal/installer"]=90
    ["internal/bridge"]=80
    ["internal/wsl"]=90
    ["internal/dotfiles"]=80
    ["pkg/manifest"]=80
    ["cmd/nexus/runner"]=75
    ["cmd/nexus"]=50
)

# ─── Functions ───

usage() {
    echo "Usage: $0 [--check|--update]"
    echo ""
    echo "  (no flags)   Check coverage and auto-update threshold if improved"
    echo "  --check      Check only; exit 1 if below threshold (CI mode)"
    echo "  --update     Force update threshold file with current coverage"
    exit 1
}

info()  { echo -e "\033[0;34mℹ\033[0m  $*"; }
pass()  { echo -e "\033[0;32m✅\033[0m  $*"; }
warn()  { echo -e "\033[0;33m⚠\033[0m  $*"; }
fail()  { echo -e "\033[0;31m❌\033[0m  $*"; }

run_coverage() {
    cd "${PROJECT_DIR}"
    info "Running test coverage analysis..."
    go test -coverprofile=coverage.out ./... > /dev/null 2>&1 || {
        fail "Tests failed. Fix test failures before checking coverage."
        exit 1
    }
}

get_total_coverage() {
    go tool cover -func=coverage.out | grep total | awk '{print $3}' | tr -d '%'
}

get_package_coverage() {
	local pkg="$1"
	# coverage.out format: "nexus-engine/PKG/file.go:line.col,line.col num_stmts count"
	# Where num_stmts is the statement count for the block and count is
	# the number of times the block was executed. A block is "covered"
	# when count > 0. Sum the statement counts for all blocks (total)
	# and for covered blocks only (covered), then compute the ratio.
	grep "^${pkg}/" coverage.out | awk '{ stmts += $2; if ($3 > 0) covered += $2 } END { if (stmts > 0) printf "%.1f", (covered/stmts)*100; else print "0" }'
}

read_threshold() {
    if [ -f "${THRESHOLD_FILE}" ]; then
        cat "${THRESHOLD_FILE}"
    else
        echo "${MINIMUM_THRESHOLD}"
    fi
}

write_threshold() {
    local value="$1"
    echo "${value}" > "${THRESHOLD_FILE}"
    info "Updated .coverage-threshold to ${value}%"
}

# ─── Main ───

MODE="auto"
while [ $# -gt 0 ]; do
    case "$1" in
        --check)  MODE="check"  ;;
        --update) MODE="update" ;;
        -h|--help) usage        ;;
        *)        usage         ;;
    esac
    shift
done

run_coverage

TOTAL_COVERAGE=$(get_total_coverage)
CURRENT_THRESHOLD=$(read_threshold)

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "  NEXUS PROTOCOL — COVERAGE RATCHET REPORT"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "  Total Coverage:    ${TOTAL_COVERAGE}%"
echo "  Current Ratchet:  ${CURRENT_THRESHOLD}%"
echo "  Absolute Minimum: ${MINIMUM_THRESHOLD}%"
echo ""

# ─── Check total coverage ───

CHECK_FAILED=0

if [ "$(echo "${TOTAL_COVERAGE} < ${MINIMUM_THRESHOLD}" | bc -l)" -eq 1 ]; then
    fail "Total coverage ${TOTAL_COVERAGE}% is below absolute minimum ${MINIMUM_THRESHOLD}%"
    CHECK_FAILED=1
elif [ "$(echo "${TOTAL_COVERAGE} < ${CURRENT_THRESHOLD}" | bc -l)" -eq 1 ]; then
    fail "Total coverage ${TOTAL_COVERAGE}% has REGRESSED below ratchet ${CURRENT_THRESHOLD}%"
    warn "This means code was removed or changed that previously had test coverage."
    CHECK_FAILED=1
else
    pass "Total coverage ${TOTAL_COVERAGE}% meets ratchet ${CURRENT_THRESHOLD}%"
fi

# ─── Check per-package coverage ───

echo ""
echo "  ── Per-Package Coverage ──────────────────────────────────────"
echo ""

for pkg in "${!PACKAGE_MINIMUMS[@]}"; do
    pkg_coverage=$(get_package_coverage "${pkg}")
    pkg_min="${PACKAGE_MINIMUMS[${pkg}]}"

    if [ -z "${pkg_coverage}" ]; then
        warn "${pkg}: no coverage data (package may not exist on this platform)"
        continue
    fi

    if [ "$(echo "${pkg_coverage} < ${pkg_min}" | bc -l)" -eq 1 ]; then
        fail "${pkg}: ${pkg_coverage}% (minimum: ${pkg_min}%)"
        CHECK_FAILED=1
    else
        pass "${pkg}: ${pkg_coverage}% (minimum: ${pkg_min}%)"
    fi
done

# ─── Determine action ───

echo ""

if [ "${CHECK_FAILED}" -eq 1 ]; then
    if [ "${MODE}" = "check" ]; then
        fail "Coverage check failed. Add tests before merging."
        exit 1
    else
        warn "Coverage is below threshold. Will NOT auto-update ratchet."
        warn "Add tests to bring coverage back up, then run this script again."
        exit 1
    fi
fi

# Coverage meets or exceeds threshold — update if improved
if [ "$(echo "${TOTAL_COVERAGE} > ${CURRENT_THRESHOLD}" | bc -l)" -eq 1 ]; then
    info "Coverage improved from ${CURRENT_THRESHOLD}% to ${TOTAL_COVERAGE}%!"

    if [ "${MODE}" = "check" ]; then
        # In CI, just report the improvement but don't write files
        pass "Coverage improved. Ratchet file should be updated locally."
    else
        # Auto-update or force-update
        write_threshold "${TOTAL_COVERAGE}"
        pass "Ratchet updated. Commit .coverage-threshold to lock in the improvement."
    fi
else
    info "Coverage stable at ${TOTAL_COVERAGE}% (ratchet: ${CURRENT_THRESHOLD}%)"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""

# Clean up
rm -f coverage.out

exit 0
