#!/usr/bin/env node
// Run madge --circular and filter out a small allowlist of known
// type-only cycles. Use case: SDK files where Host and PluginSpec
// mutually reference each other purely through `import type`, which
// TypeScript erases at compile time (zero runtime cycle), but madge —
// which doesn't distinguish type from value imports — still flags.
//
// Any cycle NOT on the allowlist fails the script. New cycles in the
// codebase will be caught immediately.

import { execFileSync } from "node:child_process";

// Each entry is the *full set of files* in a known cycle, sorted.
// A reported cycle matches if its sorted file list deep-equals one
// of these. Add a new entry only after careful review.
const ALLOWED = [
  // Host (host.ts) ↔ PluginSpec/LoadedPlugin (plugin.ts) — host.plugins.*
  // methods take a PluginSpec; PluginSpec.setup receives PluginContext
  // (which carries the Host). Both imports are `import type` — erased
  // at compile time, no runtime cycle. Restructuring would require
  // weakening type safety on the host's plugin namespace.
  ["plugins/sdk/types/host.ts", "plugins/sdk/types/plugin.ts"],
];

const allowedKeys = new Set(ALLOWED.map((cycle) => [...cycle].sort().join("|")));

let raw;
try {
  raw = execFileSync("npx", ["madge", "--circular", "--extensions", "ts,tsx", "--json", "src/"], {
    encoding: "utf8",
  });
} catch (err) {
  // madge exits non-zero when it finds any cycle. We still want the
  // JSON, which it prints on stdout regardless.
  raw = err.stdout?.toString() ?? "";
}

let cycles;
try {
  cycles = JSON.parse(raw);
} catch {
  console.error("[check-circular] madge did not produce valid JSON:");
  console.error(raw);
  process.exit(2);
}

const unexpected = cycles.filter((cycle) => !allowedKeys.has([...cycle].sort().join("|")));

if (unexpected.length > 0) {
  console.error(`[check-circular] Found ${unexpected.length} new circular dependency(ies):`);
  for (const cycle of unexpected) {
    console.error("  " + cycle.join(" > ") + " > " + cycle[0]);
  }
  console.error("");
  console.error("If a new cycle is intentional (type-only, no runtime hazard),");
  console.error("add it to ALLOWED in scripts/check-circular.mjs with a comment.");
  process.exit(1);
}

console.log(`[check-circular] OK — ${cycles.length} cycle(s) found, all on the allowlist.`);
