#!/usr/bin/env node
// Circular-dependency guard for the runtime module graph.
//
// dependency-cruiser omits TypeScript pre-compilation dependencies by default,
// so `import type` edges are erased before cycle analysis just as they are by
// the compiler. Any reported cycle therefore contains a runtime value edge and
// must be broken.

import { execFileSync } from "node:child_process";

let raw;
try {
  raw = execFileSync(
    "npx",
    [
      "--no-install",
      "dependency-cruiser",
      "--no-config",
      "--output-type",
      "json",
      "--ts-config",
      "tsconfig.json",
      "--include-only",
      "^src",
      "src",
    ],
    { encoding: "utf8", maxBuffer: 32 * 1024 * 1024 },
  );
} catch (error) {
  console.error("[check-circular] dependency analysis failed:");
  console.error(error.stderr?.toString() || error.message);
  process.exit(2);
}

let report;
try {
  report = JSON.parse(raw);
} catch {
  console.error("[check-circular] dependency-cruiser did not produce valid JSON:");
  console.error(raw);
  process.exit(2);
}

if (!Array.isArray(report.modules)) {
  console.error("[check-circular] dependency-cruiser report has no module graph");
  process.exit(2);
}

function canonicalCycle(path) {
  const nodes = path.filter(Boolean);
  if (nodes.length > 1 && nodes.at(-1) === nodes[0]) nodes.pop();

  const rotations = nodes.map((_, index) => [...nodes.slice(index), ...nodes.slice(0, index)]);
  rotations.sort((left, right) => left.join("\0").localeCompare(right.join("\0")));
  const canonical = rotations[0];
  return {
    key: canonical.join("\0"),
    path: [...canonical, canonical[0]],
  };
}

const cyclesByKey = new Map();
for (const module of report.modules) {
  for (const dependency of module.dependencies ?? []) {
    if (!dependency.circular) continue;

    const reportedPath = dependency.cycle?.map((step) => step.name) ?? [
      module.source,
      dependency.resolved,
    ];
    const cycle = canonicalCycle(reportedPath);
    cyclesByKey.set(cycle.key, cycle.path);
  }
}

const cycles = [...cyclesByKey.values()];

if (cycles.length > 0) {
  console.error(`[check-circular] Found ${cycles.length} runtime circular dependency(ies):`);
  for (const cycle of cycles) {
    console.error(`  ${cycle.join(" > ")}`);
  }
  console.error("");
  console.error(
    "Break each cycle, or make the relevant edge type-only when it carries only types.",
  );
  process.exit(1);
}

console.log("[check-circular] OK — no runtime cycles");
