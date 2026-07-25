#!/usr/bin/env node
// Design-token guard — keeps type size and corner radius on their ladders.
//
// Why this exists: before the ladders landed, the tree carried 16 distinct
// hardcoded `text-[Npx]` values across ~390 callsites and 12 distinct
// `rounded-[Npx]` values, which is how a UI ends up with 11px next to 11.5px
// and 12px next to 14px corners. Both are now derived (globals.css `--fs-*` /
// `--shape-*`), so an arbitrary pixel value at a callsite is a regression: it
// silently opts that one element out of the user's size and shape preferences.
//
// Escape hatch: none by design. A size or radius the ladder cannot express is a
// signal the ladder needs a step, not that this callsite needs an exception —
// add the step in globals.css so every other callsite can reach it too.

import { readFileSync, readdirSync, statSync } from "node:fs";
import { extname, join, relative } from "node:path";

const SRC = new URL("../src/", import.meta.url).pathname;

const MARKUP_RULES = [
  {
    pattern: /(?<![\w-])(?:[a-z-]+:)*text-\[[\d.]+px\]/g,
    message: "arbitrary font size — use a `text-ui-*` / `text-display-*` step",
  },
  {
    pattern: /(?<![\w-])(?:[a-z-]+:)*rounded(?:-[trbl]{1,2})?-\[[\d.]+(?:px|rem)\]/g,
    message: "arbitrary corner radius — use a `rounded-*` step",
  },
];

const STYLESHEET_RULES = [
  {
    pattern: /font-size:\s*[\d.]+(?:px|rem)/g,
    message: "arbitrary font size — use `var(--fs-*)`",
  },
  {
    pattern: /border-radius:\s*[\d.]+(?:px|rem)/g,
    message: "arbitrary corner radius — use `var(--shape-*)`",
  },
];

// globals.css owns the ladders, so it is the one file allowed to spell the
// numbers out.
const STYLESHEET_EXEMPT = new Set(["styles/globals.css"]);

function* walk(dir) {
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) yield* walk(path);
    else yield path;
  }
}

function rulesFor(path) {
  const ext = extname(path);
  if (ext === ".tsx" || ext === ".ts") return MARKUP_RULES;
  if (ext === ".css") return STYLESHEET_EXEMPT.has(relative(SRC, path)) ? [] : STYLESHEET_RULES;
  return [];
}

const violations = [];
for (const path of walk(SRC)) {
  const rules = rulesFor(path);
  if (rules.length === 0) continue;
  const lines = readFileSync(path, "utf8").split("\n");
  lines.forEach((line, index) => {
    for (const { pattern, message } of rules) {
      for (const match of line.matchAll(pattern)) {
        violations.push(`${relative(SRC, path)}:${index + 1}  ${match[0]}  — ${message}`);
      }
    }
  });
}

if (violations.length > 0) {
  console.error(`check-design-tokens: ${violations.length} off-ladder value(s)\n`);
  for (const violation of violations) console.error(`  ${violation}`);
  process.exit(1);
}
console.log("check-design-tokens: type + radius ladders clean");
