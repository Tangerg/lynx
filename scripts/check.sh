#!/usr/bin/env bash
# Local check runner — same set of checks CI runs, easy to invoke
# before pushing.
#
# Usage:
#   scripts/check.sh                       # run everything
#   scripts/check.sh build vet test        # subset
#   FAST=1 scripts/check.sh                # skip govulncheck (slowest)
#   MODULE=core scripts/check.sh           # single workspace module only
#   MODULE=app/runtime scripts/check.sh race
#
# Required tools:
#   go (1.26.5)
#   golangci-lint  — install via:
#     go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
#   govulncheck    — install via:
#     go install golang.org/x/vuln/cmd/govulncheck@latest
set -euo pipefail

cd "$(dirname "$0")/.."

ROOT=$PWD
MODULES=()
while IFS= read -r module_dir; do
  [[ -z "$module_dir" ]] && continue
  MODULES+=("${module_dir#"$ROOT"/}")
done < <(go list -m -f '{{if .Main}}{{.Dir}}{{end}}' all)

if [[ ${#MODULES[@]} -eq 0 ]]; then
  echo "no main modules found in go.work" >&2
  exit 1
fi

# Override module list via env (single module spot-check).
if [[ -n "${MODULE:-}" ]]; then
  if [[ ! -f "$MODULE/go.mod" ]]; then
    echo "unknown workspace module: $MODULE" >&2
    exit 2
  fi
  MODULES=("$MODULE")
fi

# Checks to run; default = all.
if [[ $# -eq 0 ]]; then
  CHECKS=(build vet test lint vuln)
else
  CHECKS=("$@")
fi

# FAST skips the slowest checks (govulncheck hits the net).
if [[ "${FAST:-0}" == "1" ]]; then
  FAST_CHECKS=()
  for check in "${CHECKS[@]}"; do
    [[ "$check" == "vuln" ]] || FAST_CHECKS+=("$check")
  done
  CHECKS=("${FAST_CHECKS[@]}")
fi

run_in_module() {
  local mod=$1
  local check=$2
  echo "── $mod ── $check"
  case "$check" in
    build) (cd "$mod" && go build ./...) ;;
    vet)   (cd "$mod" && go vet ./...) ;;
    test)  (cd "$mod" && go test ./...) ;;
    race)  (cd "$mod" && go test -race ./...) ;;
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
