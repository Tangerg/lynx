#!/usr/bin/env node
// Circular-dependency guard. The rule it enforces is "no cycle at RUNTIME" —
// a module graph that can't be initialised in any order.
//
// madge reports the import graph without distinguishing `import type` from a
// value import, and a type-only edge doesn't exist at runtime: TypeScript erases
// it. So every cycle it finds has to be classified, and that used to be a
// hand-maintained allowlist of file sets — which drifts. One of its two entries
// named four files, three of which no longer existed; it had been passing on a
// cycle that could no longer happen.
//
// So the classification is derived instead: resolve every edge of a reported
// cycle back to its import statement, and if all of them are type-only the cycle
// is erased at compile time and benign. Any cycle with even one value edge fails.
// No list to maintain, and a genuinely new runtime cycle fails on the commit
// that introduces it.

import { execFileSync } from "node:child_process";
import { existsSync, readFileSync } from "node:fs";
import { dirname, join, relative, resolve } from "node:path";

const SRC = resolve("src");

let raw;
try {
  raw = execFileSync(
    "npx",
    [
      "madge",
      "--circular",
      "--extensions",
      "ts,tsx",
      "--ts-config",
      "tsconfig.json",
      "--json",
      "src/",
    ],
    { encoding: "utf8" },
  );
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

// Every import statement in a file, as { specifier, typeOnly }. `import type {…}`
// and `import { type X }` are both erased; a mixed statement keeps a value edge.
function importsOf(file) {
  const source = readFileSync(join(SRC, file), "utf8");
  const out = [];
  const pattern = /import\s+(type\s+)?([\s\S]*?)from\s*["']([^"']+)["']/g;
  for (const [, typeKeyword, clause, specifier] of source.matchAll(pattern)) {
    const named = clause.match(/\{([\s\S]*)\}/)?.[1];
    const allNamedAreTypes =
      named !== undefined &&
      named
        .split(",")
        .map((part) => part.trim())
        .filter(Boolean)
        .every((part) => part.startsWith("type "));
    out.push({ specifier, typeOnly: Boolean(typeKeyword) || allNamedAreTypes });
  }
  return out;
}

// A specifier as written resolved to a src-relative path, or null when it points
// outside src (a package) or can't be resolved.
function resolveSpecifier(fromFile, specifier) {
  const base = specifier.startsWith("@/")
    ? join(SRC, specifier.slice(2))
    : specifier.startsWith(".")
      ? join(SRC, dirname(fromFile), specifier)
      : null;
  if (base === null) return null;
  for (const candidate of [
    base,
    `${base}.ts`,
    `${base}.tsx`,
    join(base, "index.ts"),
    join(base, "index.tsx"),
  ]) {
    if (existsSync(candidate) && !candidate.endsWith("/")) {
      const rel = relative(SRC, candidate);
      if (!rel.startsWith("..")) return rel;
    }
  }
  return null;
}

function valueEdges(cycle) {
  const edges = [];
  for (let index = 0; index < cycle.length; index += 1) {
    const from = cycle[index];
    const to = cycle[(index + 1) % cycle.length];
    const matching = importsOf(from).filter(
      (entry) => resolveSpecifier(from, entry.specifier) === to,
    );
    // An edge madge saw but this can't resolve counts as a value edge: better a
    // false failure that gets read than a cycle waved through.
    if (matching.length === 0 || matching.some((entry) => !entry.typeOnly)) {
      edges.push(`${from} → ${to}`);
    }
  }
  return edges;
}

const runtimeCycles = cycles
  .map((cycle) => ({ cycle, edges: valueEdges(cycle) }))
  .filter(({ edges }) => edges.length > 0);

if (runtimeCycles.length > 0) {
  console.error(`[check-circular] Found ${runtimeCycles.length} runtime circular dependency(ies):`);
  for (const { cycle, edges } of runtimeCycles) {
    console.error("  " + cycle.join(" > ") + " > " + cycle[0]);
    for (const edge of edges) console.error(`      value edge: ${edge}`);
  }
  console.error("");
  console.error("Break the cycle, or make the edge type-only if the import is a type.");
  process.exit(1);
}

const typeOnly = cycles.length;
console.log(
  typeOnly === 0
    ? "[check-circular] OK — no cycles"
    : `[check-circular] OK — no runtime cycles (${typeOnly} type-only, erased at compile time)`,
);
