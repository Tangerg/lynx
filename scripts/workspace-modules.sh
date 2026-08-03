#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)

while IFS= read -r module_dir; do
  [[ -z "$module_dir" ]] && continue
  case "$module_dir" in
    "$root")
      printf '.\n'
      ;;
    "$root"/*)
      printf '%s\n' "${module_dir#"$root"/}"
      ;;
    *)
      printf 'workspace module is outside repository: %s\n' "$module_dir" >&2
      exit 1
      ;;
  esac
done < <(cd "$root" && go list -m -f '{{if .Main}}{{.Dir}}{{end}}' all)
