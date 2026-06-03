#!/usr/bin/env bash
# Local check runner — same set of checks CI runs, easy to invoke
# before pushing.
#
# Usage:
#   scripts/check.sh                       # run everything
#   scripts/check.sh build vet test        # subset
#   FAST=1 scripts/check.sh                # skip govulncheck (slowest)
#   MODULE=lyra scripts/check.sh           # single module only
#
# Required tools:
#   go (1.26.3)
#   golangci-lint  — install via:
#     go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
#   govulncheck    — install via:
#     go install golang.org/x/vuln/cmd/govulncheck@latest
set -euo pipefail

cd "$(dirname "$0")/.."

# All go.mod-bearing directories under the workspace, in dependency-
# friendly order (pkg / core first so downstream modules pick up
# fresh artifacts). Mirrors go.work `use (...)` entries plus the
# three documentreaders sub-modules.
MODULES=(
  pkg
  core
  agent
  chatmemory
  documentreaders
  documentreaders/html
  documentreaders/markdown
  documentreaders/pdf
  mcp
  models
  otel
  rag
  tools
  vectorstores
  lyra
)

# Override module list via env (single module spot-check).
if [[ -n "${MODULE:-}" ]]; then
  MODULES=("$MODULE")
fi

# Checks to run; default = all.
CHECKS=("${@:-build vet test lint vuln}")
if [[ ${#CHECKS[@]} -eq 1 && "${CHECKS[0]}" == *" "* ]]; then
  # Single string with spaces — split to array.
  read -ra CHECKS <<< "${CHECKS[0]}"
fi

# FAST skips the slowest checks (govulncheck hits the net).
if [[ "${FAST:-0}" == "1" ]]; then
  CHECKS=("${CHECKS[@]/vuln}")
fi

run_in_module() {
  local mod=$1
  local check=$2
  echo "── $mod ── $check"
  case "$check" in
    build) (cd "$mod" && go build ./...) ;;
    vet)   (cd "$mod" && go vet ./...) ;;
    test)  (cd "$mod" && go test -race ./...) ;;
    lint)  (cd "$mod" && golangci-lint run ./...) ;;
    vuln)  (cd "$mod" && govulncheck ./...) ;;
    *) echo "unknown check: $check" >&2; return 2 ;;
  esac
}

failed=()
for mod in "${MODULES[@]}"; do
  for check in "${CHECKS[@]}"; do
    [[ -z "$check" ]] && continue
    if ! run_in_module "$mod" "$check"; then
      failed+=("$mod/$check")
    fi
  done
done

if [[ ${#failed[@]} -gt 0 ]]; then
  echo
  echo "FAILED:"
  printf '  %s\n' "${failed[@]}"
  exit 1
fi

echo
echo "all green ($((${#MODULES[@]} * ${#CHECKS[@]})) checks)"
