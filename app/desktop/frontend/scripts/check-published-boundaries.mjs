#!/usr/bin/env node
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";

const SRC = join(process.cwd(), "src");
const TEXT_EXT = /\.(ts|tsx|md)$/;

function files(dir) {
  const out = [];
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    const stat = statSync(path);
    if (stat.isDirectory()) out.push(...files(path));
    else if (TEXT_EXT.test(path)) out.push(path);
  }
  return out;
}

const violations = [];

for (const file of files(SRC)) {
  const rel = relative(SRC, file);
  const text = readFileSync(file, "utf8");

  if (/@\/protocol\/run|protocol\/run|agent\/core-reducer|core-reducer/.test(text)) {
    violations.push({
      file: rel,
      reason: "old agent fold/view-state path or name is referenced",
    });
  }

  if (/plugins\/builtin\/chat\/composer\/public\/input\.ts$/.test(rel) && /@\/rpc/.test(text)) {
    violations.push({
      file: rel,
      reason: "composer public input must expose composer language, not runtime wire types",
    });
  }

  if (
    /plugins\/builtin\/.+\/public\/.+\.(ts|tsx)$/.test(rel) &&
    /from\s+["']@\/rpc["']/.test(text)
  ) {
    violations.push({
      file: rel,
      reason: "builtin public surfaces must not import runtime wire directly",
    });
  }
}

if (violations.length > 0) {
  console.error(`[check-published-boundaries] Found ${violations.length} violation(s):`);
  for (const violation of violations) console.error(`  ${violation.file}: ${violation.reason}`);
  process.exit(1);
}

console.log("[check-published-boundaries] OK — published boundaries stay wire-free.");
