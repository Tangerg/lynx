#!/usr/bin/env bash
# Execute the Core v1 code, compatibility, provider, backend, and fuzz gates.
# Fuzz targets run for five minutes each by default; set FUZZ_TIME=0 only when
# reproducing the non-fuzz portion after a separately recorded full fuzz run.
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
fuzz_time=${FUZZ_TIME:-5m}

(
  cd "$root/core"
  go test -count=1 ./...
  go test -race -count=1 ./...
  go vet ./...
  golangci-lint run --config="$root/.golangci.yml" ./...
  go mod tidy -diff
)

"$root/scripts/check-core-coverage.sh"

(
  cd "$root/dev/providerconformance"
  go test -race -count=1 ./...
)

(
  cd "$root/models"
  go test -race -count=1 ./anthropic ./bedrock ./google ./ollama ./openai
)

vectorstore_packages=()
while IFS= read -r module_file; do
  vectorstore_packages+=("./$(basename "$(dirname "$module_file")")/...")
done < <(find "$root/vectorstores" -mindepth 2 -maxdepth 2 -name go.mod -print | sort)
if [[ ${#vectorstore_packages[@]} -eq 0 ]]; then
  echo "no vectorstore modules found" >&2
  exit 1
fi
(
  cd "$root/vectorstores"
  go test -race -count=1 "${vectorstore_packages[@]}" -run '^TestStoreConformance$'
)

if [[ "$fuzz_time" != "0" ]]; then
  fuzz() {
    local package=$1
    local target=$2
    (
      cd "$root/core"
      go test "$package" -run '^$' -fuzz "^${target}$" -fuzztime "$fuzz_time"
    )
  }

  fuzz ./metadata FuzzMapJSON
  fuzz ./media FuzzMediaJSON
  fuzz ./vectorstore/filter FuzzParse
  fuzz ./chat FuzzPartJSON
  fuzz ./chat FuzzMessageJSON
  fuzz ./chat FuzzRequestJSON
  fuzz ./chat FuzzResponseJSON
else
  echo "fuzz targets skipped because FUZZ_TIME=0"
fi

echo "Core v1 release gates passed"
