#!/usr/bin/env bash
# Local check runner — same set of checks CI runs, easy to invoke
# before pushing.
#
# Usage:
#   scripts/check.sh                       # run everything
#   scripts/check.sh build vet test        # subset
#   FAST=1 scripts/check.sh                # skip govulncheck (slowest)
#   MODULE=. scripts/check.sh              # root workspace module only
#   MODULE=models/google scripts/check.sh  # nested workspace module only
#   MODULE=app/runtime scripts/check.sh race
#
# Required tools:
#   go (1.27.0)
#   golangci-lint  — install via:
#     go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
#   govulncheck    — install via:
#     go install golang.org/x/vuln/cmd/govulncheck@v1.6.0
#   jq              — used to enforce the reviewed vulnerability allowlist
#   node/npm         — required only when app/desktop is selected
set -euo pipefail

cd "$(dirname "$0")/.."

ROOT=$PWD
MODULES=()
while IFS= read -r module; do
  [[ -z "$module" ]] || MODULES+=("$module")
done < <(scripts/workspace-modules.sh)

if [[ ${#MODULES[@]} -eq 0 ]]; then
  echo "no main modules found in go.work" >&2
  exit 1
fi

# Override module list via env (single module spot-check).
if [[ -n "${MODULE:-}" ]]; then
  known=0
  for workspace_module in "${MODULES[@]}"; do
    if [[ "$workspace_module" == "$MODULE" ]]; then
      known=1
      break
    fi
  done
  if [[ $known -eq 0 ]]; then
    echo "unknown workspace module: $MODULE" >&2
    exit 2
  fi
  MODULES=("$MODULE")
fi

# Checks to run; default = all.
if [[ $# -eq 0 ]]; then
  CHECKS=(build vet test tidy lint vuln)
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

# The Wails binary embeds frontend/dist. Build that owned input before any Go
# check that loads packages; a clean checkout deliberately does not track dist.
prepare_desktop=0
for module in "${MODULES[@]}"; do
  [[ "$module" == "app/desktop" ]] || continue
  for check in "${CHECKS[@]}"; do
    if [[ "$check" != "tidy" ]]; then
      prepare_desktop=1
      break 2
    fi
  done
done
if [[ $prepare_desktop -eq 1 ]]; then
  if ! command -v npm >/dev/null 2>&1; then
    echo "npm is required to build app/desktop frontend assets" >&2
    exit 2
  fi
  if [[ ! -d "$ROOT/app/desktop/frontend/node_modules" ]]; then
    echo "app/desktop frontend dependencies are missing; run npm ci in app/desktop/frontend" >&2
    exit 2
  fi
  (cd "$ROOT/app/desktop/frontend" && npm run build)
fi

run_in_module() {
  local mod=$1
  local check=$2
  echo "── $mod ── $check"
  case "$check" in
    build)
      if [[ ${#MODULE_BUILD_PACKAGES[@]} -eq 1 ]] &&
        [[ $(cd "$mod" && go list -f '{{.Name}}' "${MODULE_BUILD_PACKAGES[0]}") == "main" ]]; then
        (cd "$mod" && go build -o /dev/null "${MODULE_BUILD_PACKAGES[0]}")
      else
        (cd "$mod" && go build "${MODULE_BUILD_PACKAGES[@]}")
      fi
      ;;
    vet)   (cd "$mod" && go vet "${MODULE_PACKAGES[@]}") ;;
    test)  (cd "$mod" && go test -count=1 "${MODULE_PACKAGES[@]}") ;;
    race)  (cd "$mod" && go test -race -count=1 "${MODULE_PACKAGES[@]}") ;;
    tidy)  (cd "$mod" && go mod tidy -diff) ;;
    lint)  (cd "$mod" && golangci-lint run --config="$ROOT/.golangci.yml" ./...) ;;
    vuln)  "$ROOT/scripts/check-vulnerabilities.sh" "$mod" ;;
    *) echo "unknown check: $check" >&2; return 2 ;;
  esac
}

failed=()
for mod in "${MODULES[@]}"; do
  MODULE_PACKAGES=()
  MODULE_BUILD_PACKAGES=()
  needs_packages=0
  needs_buildable_packages=0
  for check in "${CHECKS[@]}"; do
    case "$check" in
      build|vet|test|race) needs_packages=1 ;;
    esac
    [[ "$check" == "build" ]] && needs_buildable_packages=1
  done
  if [[ $needs_packages -eq 1 ]]; then
    while IFS= read -r package; do
      [[ -z "$package" ]] || MODULE_PACKAGES+=("$package")
    done < <(scripts/module-packages.sh "$mod")
    if [[ ${#MODULE_PACKAGES[@]} -eq 0 ]]; then
      echo "$mod: no Go packages found" >&2
      failed+=("$mod/packages")
      continue
    fi
    if [[ $needs_buildable_packages -eq 1 ]]; then
      while IFS= read -r package; do
        [[ -z "$package" ]] || MODULE_BUILD_PACKAGES+=("$package")
      done < <(scripts/module-packages.sh --buildable "$mod")
      if [[ ${#MODULE_BUILD_PACKAGES[@]} -eq 0 ]]; then
        echo "$mod: no buildable Go packages found" >&2
        failed+=("$mod/buildable-packages")
        continue
      fi
    fi
  fi
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
