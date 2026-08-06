#!/usr/bin/env node
// Style-invalidation guard — a custom property written by a STATE selector must be
// registered `inherits: false`.
//
// WHY THIS EXISTS: the other guards prove structure and prove nothing about cost. The
// defect that motivated this one passed every single one of them — right layer, right
// token, right vocabulary, one value for the thing it described:
//
//     .agent-shell[data-sidebar="collapsed"] .agent-content-card {
//       --shell-boundary: transparent;
//     }
//
// Custom properties inherit. So changing a half-pixel hairline that only the content
// card itself reads put every descendant of that card — the whole transcript, the
// composer, the dock — into style recalculation, and collapsing the drawer cost 37ms of
// it, landing as a dropped frame at the start of the slide. Registering the property
// non-inheriting took the same toggle to 4ms.
//
// The shape generalises, which is why it is worth a gate rather than a fixed comment:
// any `:hover` / `:focus-within` / `[data-…]` / `:has()` rule that writes a variable is
// paying for a subtree it usually does not style. The right-hand dock never had the
// problem because it keeps its state in an attribute ON the element it styles.
//
// WHAT TO DO when this fires — two honest answers, and the guard cannot pick for you:
//   * The property is element-local (written and read by the same element). Register it
//     in the "Element-local state colours" block in globals.css. Free, and the fix.
//   * A DESCENDANT genuinely reads it. Then inheritance is the mechanism and registering
//     would break it — so the write itself is the cost. Move the state up to the element
//     that owns the subtree, or express the state as an attribute the way the dock does.
//
// Scope: `globals.css` and the three imported sheets. `:root` / `@theme` / `html.theme-*`
// blocks are exempt — those declare the palette, and a theme switch repainting the
// document is what a theme switch IS.

import { readFileSync } from "node:fs";

const SHEETS = [
  "src/styles/globals.css",
  "src/styles/markdown.css",
  "src/styles/overlays.css",
  "src/styles/tool.css",
];
const ROOT = new URL("../", import.meta.url).pathname;

// A selector that changes as the user interacts, or as application state flips an
// attribute — the ones whose writes happen while someone is watching.
const STATEFUL = /:hover|:focus|:active|:checked|:disabled|:has\(|\[data-[a-z-]+/;

// Declaring the palette, not reacting to state.
const PALETTE_SCOPE = /^(?::root|html|\*|body|@theme|@property|@media|@supports|@layer)/;

const registered = new Set();
const writes = [];

for (const sheet of SHEETS) {
  let text;
  try {
    text = readFileSync(ROOT + sheet, "utf8");
  } catch {
    continue; // an imported sheet may be deleted; other guards notice that
  }
  for (const match of text.matchAll(/@property\s+(--[\w-]+)\s*\{([^}]*)\}/g)) {
    if (/inherits:\s*false/.test(match[2])) registered.add(match[1]);
  }

  // Walk blocks, tracking the selector that opened the innermost one.
  const stack = [];
  text.split("\n").forEach((line, index) => {
    const trimmed = line.trim();
    if (trimmed.endsWith("{")) {
      stack.push(trimmed.slice(0, -1).trim());
      return;
    }
    if (trimmed === "}") {
      stack.pop();
      return;
    }
    const write = /^(--[\w-]+)\s*:/.exec(trimmed);
    if (!write || stack.length === 0) return;
    const selector = stack[stack.length - 1];
    if (PALETTE_SCOPE.test(selector)) return;
    if (!STATEFUL.test(selector)) return;
    writes.push({ sheet, line: index + 1, property: write[1], selector });
  });
}

const unregistered = writes.filter((write) => !registered.has(write.property));

if (unregistered.length > 0) {
  console.error(
    `check-style-invalidation: ${unregistered.length} state-driven write(s) to an inheriting custom property\n`,
  );
  for (const { sheet, line, property, selector } of unregistered) {
    console.error(`  ${sheet}:${line}  ${property}  on  ${selector.slice(0, 72)}`);
  }
  console.error("");
  console.error("An inheriting property written by a state selector recalculates the whole");
  console.error("subtree below it. Register it non-inheriting in globals.css, or move the");
  console.error("state onto the element it styles — see the header of this script.");
  process.exit(1);
}

console.log(
  `check-style-invalidation: ${writes.length} state-driven custom-property write(s), all non-inheriting`,
);
