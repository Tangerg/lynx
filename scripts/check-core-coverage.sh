#!/usr/bin/env bash
# Enforce the P7 Core statement-coverage budget package by package. Thresholds
# come from the immutable P0 baseline, except the new protocol/serialization
# packages and public filter facade, whose architecture target is 85%.
set -euo pipefail

cd "$(dirname "$0")/../core"

failed=0
while read -r package minimum; do
  [[ -z "$package" ]] && continue
  if ! output=$(go test -count=1 -cover "$package" 2>&1); then
    echo "$output" >&2
    exit 1
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
done <<'EOF'
./chat 85.0
./document 29.2
./embedding 61.0
./image 42.3
./media 85.0
./metadata 85.0
./internal/ptr 100.0
./moderation 46.5
./speech 47.8
./transcription 42.1
./vectorstore 78.4
./vectorstore/filter 85.0
EOF

if [[ $failed -ne 0 ]]; then
  echo "Core coverage budget failed" >&2
  exit 1
fi

echo "Core coverage budget passed"
