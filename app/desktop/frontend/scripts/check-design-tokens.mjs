#!/usr/bin/env node
// Design-token guard — keeps type size, line height and corner radius on their
// ladders.
//
// Why this exists: before the ladders landed, the tree carried 16 distinct
// hardcoded `text-[Npx]` values across ~390 callsites, 12 distinct
// `rounded-[Npx]` values, and five line heights within 0.2 of each other
// (1.4 / 1.45 / 1.5 / 1.55 / 1.6) across 53 callsites. That is how a UI ends up
// with 11px next to 11.5px. All three are now derived in globals.css (`--fs-*` /
// `--leading-*` / `--shape-*`), so an arbitrary value at a callsite is a
// regression: it silently opts that one element out of the ladder, and out of the
// user's size and shape preferences.
//
// Escape hatch: none by design. A size, radius or tint the ladder cannot express is a
// signal the ladder needs a step, not that this callsite needs an exception —
// add the step in globals.css so every other callsite can reach it too.

import { readFileSync, readdirSync, statSync } from "node:fs";
import { extname, join, relative } from "node:path";

const SRC = new URL("../src/", import.meta.url).pathname;

const MARKUP_RULES = [
  {
    // `bg-negative/12`, `bg-warning/[0.06]` — a semantic tint with a hand-picked
    // alpha. Negative alone had reached five of them across the tree, so the same
    // "this is an error" answered at five strengths depending on the card.
    pattern: /(?<![\w-])(?:[a-z-]+:)*bg-(?:negative|warning|success|info|accent)\/\[?[\d.]+\]?/g,
    message:
      "hand-picked tone alpha — use `bg-<tone>-wash` (a tinted surface) or `bg-<tone>-badge` (a chip on one)",
  },
  {
    pattern: /(?<![\w-])(?:[a-z-]+:)*text-\[[\d.]+px\]/g,
    message: "arbitrary font size — use a `text-ui-*` / `text-display-*` step",
  },
  {
    // A shadow spelled out at the call site. Depth is a material, and a material
    // has to answer to the theme: six callsites had drawn their own, and one of
    // them — a selected tab lifted by `inset 0 1px 0 rgba(255,255,255,0.03)` —
    // was a value chosen for the dark theme, so in light the tab had no lift at
    // all. Two others were the same accent glow written twice.
    // `shadow-[var(--shadow-*)]` stays legal: that IS the ladder.
    pattern: /(?<![\w-])shadow-\[(?!var\(--shadow-)[^\]]*\]/g,
    message: "hardcoded shadow — define a `--shadow-*` token in globals.css and use it",
  },
  {
    // An edge alpha mixed by hand where the field tokens exist. Same failure as
    // the tone rule above: the segmented control had drawn its well at 7% and its
    // selected chip at 5%, two hand-picked values for one idea, and neither
    // followed the contrast preference (`--color-border` does).
    pattern: /(?<![\w-])(?:[a-z-]+:)*border-fg\/\[?[\d.]+\]?/g,
    message: "hand-picked edge alpha — use `border-field` / `border-field-strong`",
  },
  {
    pattern: /(?<![\w-])(?:[a-z-]+:)*rounded(?:-[trbl]{1,2})?-\[[\d.]+(?:px|rem)\]/g,
    message: "arbitrary corner radius — use a `rounded-*` step",
  },
  {
    pattern: /(?<![\w-])(?:[a-z-]+:)*leading-\[[\d.]+\]/g,
    message: "arbitrary line height — use a `leading-*` step",
  },
  {
    // An ink or accent wash mixed by hand in an arbitrary value. The mermaid
    // block had built two panels this way, out of four alphas of its own — which
    // also opted them out of the contrast preference, since `--depth-step` is
    // what moves the ladder.
    pattern: /color-mix\([^)]*var\(--color-(?:text|accent)\)[^)]*\)/g,
    message:
      "hand-mixed token alpha — use a ladder step (`bg-surface*` / `border-field*` / `bg-<tone>-wash`)",
    // The theme kit is where the ladder's values are authored — mixing is its job.
    appliesTo: (rel) => !rel.startsWith("plugins/builtin/theme/"),
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
  {
    pattern: /line-height:\s*[\d.]+\s*;/g,
    message: "arbitrary line height — use `var(--leading-*)`",
  },
  {
    // A literal colour in a consuming stylesheet answers "which colour" where the
    // theme should: it can't follow a scheme, a contributed theme, or contrast.
    // The active search hit had painted `color: #000` this way.
    pattern: /#(?:[\da-fA-F]{3,4}|[\da-fA-F]{6}|[\da-fA-F]{8})\b/g,
    message: "literal colour — define a token in globals.css and use `var(--color-*)`",
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
    for (const { pattern, message, appliesTo } of rules) {
      if (appliesTo && !appliesTo(relative(SRC, path))) continue;
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
console.log(
  "check-design-tokens: type + leading + radius + tone + colour + depth + edge ladders clean",
);
