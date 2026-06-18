#!/usr/bin/env node
// Bundle size budget — guards against accidental regressions in the
// first-paint payload (entry JS/CSS referenced directly from
// index.html). Lazy chunks (Shiki language grammars, OTEL SDK,
// per-plugin code-split) are intentionally excluded — the user only
// pays for those when the feature is exercised, so growth there is
// expected as the feature surface widens.
//
// Method:
//   1. Parse dist/index.html, extract every <script src> and
//      <link rel="stylesheet" href> that points into /assets/.
//   2. For each referenced file, measure gzip size (Node builtin zlib).
//   3. Sum + compare against the per-extension budgets in BUDGETS.
//   4. Exit non-zero on any overrun; print a focused diff vs. budget.
//
// Bumping the budget: when a deliberate increase is needed (new
// feature added to the entry path), update the constants below in
// the same commit that introduces the growth. Reviewers SHOULD push
// back on bumps without a justification line.

import { readFileSync, statSync } from "node:fs";
import { join } from "node:path";
import { gzipSync } from "node:zlib";

const DIST = "dist";
const INDEX_HTML = join(DIST, "index.html");

// Budgets in BYTES (gzipped). Recorded 2026-05-28 from a clean
// `npm run build`. Headroom is ~10–15 % above current — tight enough
// to flag a 100 KB regression, loose enough that adding a normal
// feature doesn't bump it.
const BUDGETS = {
  js: 960_000, // entry JS (current ~858 KB gzip)
  css: 25_000, // entry CSS (current ~14.5 KB gzip)
};

function loadIndexHtml() {
  try {
    return readFileSync(INDEX_HTML, "utf8");
  } catch (err) {
    console.error(`[check-bundle-size] ${INDEX_HTML} not found — run \`npm run build\` first.`);
    console.error(err.message);
    process.exit(2);
  }
}

function extractEntryAssets(html) {
  const js = [...html.matchAll(/<script[^>]*\ssrc="(\/assets\/[^"]+\.js)"/g)].map((m) => m[1]);
  const css = [
    ...html.matchAll(/<link[^>]*\srel="stylesheet"[^>]*\shref="(\/assets\/[^"]+\.css)"/g),
    ...html.matchAll(/<link[^>]*\shref="(\/assets\/[^"]+\.css)"[^>]*\srel="stylesheet"/g),
  ].map((m) => m[1]);
  return { js, css };
}

function gzipSizeOf(relativeUrl) {
  // index.html references are absolute (/assets/...). Strip the leading
  // slash to make a filesystem path relative to dist/.
  const path = join(DIST, relativeUrl.replace(/^\//, ""));
  statSync(path); // throw early with a clear message if missing
  return gzipSync(readFileSync(path)).length;
}

function formatKb(bytes) {
  return `${(bytes / 1024).toFixed(1)} KB`;
}

const html = loadIndexHtml();
const { js, css } = extractEntryAssets(html);

if (js.length === 0) {
  console.error("[check-bundle-size] no <script> entry found in index.html");
  process.exit(2);
}

const totals = {
  js: js.reduce((sum, url) => sum + gzipSizeOf(url), 0),
  css: css.reduce((sum, url) => sum + gzipSizeOf(url), 0),
};

let failed = false;
console.log("[check-bundle-size] entry payload (gzip):");
for (const ext of /** @type {const} */ (["js", "css"])) {
  const used = totals[ext];
  const budget = BUDGETS[ext];
  const pct = ((used / budget) * 100).toFixed(1);
  const status = used > budget ? "FAIL" : "OK";
  if (used > budget) failed = true;
  console.log(
    `  ${ext.toUpperCase().padEnd(4)} ${formatKb(used).padStart(10)} / ${formatKb(budget).padStart(10)}  (${pct}%) ${status}`,
  );
}

if (failed) {
  console.error("");
  console.error("[check-bundle-size] FAIL — entry payload exceeded its budget.");
  console.error("If this growth is intentional, update BUDGETS in");
  console.error("scripts/check-bundle-size.mjs in the same commit.");
  process.exit(1);
}
