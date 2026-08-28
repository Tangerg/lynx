#!/usr/bin/env bash
# Enforce a ratcheted Core statement-coverage budget package by package. Every
# public package must be listed, including packages with no statements, so a
# new package cannot silently bypass the gate. Selected internal owners with
# executable behavior are covered by the same budget.
set -euo pipefail

cd "$(dirname "$0")/../core"

coverage_budget=(
  "./chat 89.1"
  "./chatclient 88.8"
  "./chatclient/safeguard 88.5"
  "./document 89.7"
  "./embedding 80.6"
  "./embeddingclient 91.9"
  "./history 88.9"
  "./history/inmemory 93.8"
  "./history/storetest 84.3"
  "./image 63.4"
  "./internal/extension 83.0"
  "./internal/ptr 100.0"
  "./jsonschema 73.0"
  "./media 91.9"
  "./metadata 85.0"
  "./modeltest 13.6"
  "./moderation 55.3"
  "./speech 58.5"
  "./tokenizer none"
  "./tool 91.1"
  "./transcription 56.9"
  "./vectorstore 84.5"
  "./vectorstore/filter 85.0"
  "./vectorstore/inmemory 68.0"
  "./vectorstore/storetest 59.4"
)

configured_packages=$(
  for budget in "${coverage_budget[@]}"; do
    read -r package _ <<<"$budget"
    printf '%s\n' "$package"
  done | sort
)
tracked_packages=$(
  {
    go list ./... |
      sed 's#^github.com/Tangerg/scope/core#.#' |
      awk '$0 !~ /\/internal(\/|$)/'
    printf '%s\n' ./internal/extension ./internal/ptr
  } | sort
)
if [[ "$configured_packages" != "$tracked_packages" ]]; then
  echo "Core coverage budget package inventory is stale" >&2
  diff -u <(printf '%s\n' "$configured_packages") <(printf '%s\n' "$tracked_packages") >&2 || true
  exit 1
fi

failed=0
for budget in "${coverage_budget[@]}"; do
  read -r package minimum <<<"$budget"
  if ! output=$(go test -count=1 -cover "$package" 2>&1); then
    echo "$output" >&2
    exit 1
  fi
  if [[ "$minimum" == "none" ]]; then
    if [[ "$output" != *"coverage: [no statements]"* ]]; then
      echo "$package gained executable statements; assign it a coverage budget" >&2
      echo "$output" >&2
      failed=1
    else
      printf '%-43s %s\n' "$package" "no statements"
    fi
    continue
  fi
  actual=$(printf '%s\n' "$output" | sed -n 's/.*coverage: \([0-9][0-9.]*\)% of statements.*/\1/p')
  if [[ -z "$actual" ]]; then
    echo "could not read coverage for $package from: $output" >&2
    exit 1
  fi
  printf '%-43s %6s%% (minimum %s%%)\n' "$package" "$actual" "$minimum"
  if ! awk -v actual="$actual" -v minimum="$minimum" 'BEGIN { exit !(actual + 0 >= minimum + 0) }'; then
    failed=1
  fi
done

if [[ $failed -ne 0 ]]; then
  echo "Core coverage budget failed" >&2
  exit 1
fi

echo "Core coverage budget passed"
