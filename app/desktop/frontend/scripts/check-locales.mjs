#!/usr/bin/env node
// Locale-catalog guard — every locale carries every key, exactly once.
//
// Why this exists: the catalogs are typed `Record<string, string>`, so a locale
// that never learned a key is not a compile error — it is a silent fall back to
// English at runtime. Nothing was watching, and the gap had grown to 253 of 765
// keys for five languages (a third of the app in English for a German user), 235
// for zh-TW and 70 for zh. The gap is closed; this keeps it closed.
//
// Three rules, all hard, all at zero:
//
//   1. No catalog declares a key twice. A duplicate is legal JS and silently
//      keeps the last one, so the earlier entry looks translated while doing
//      nothing. That is how `diagnostics.clear` came to exist in all eight
//      catalogs while the button beside it rendered a hardcoded "Clear".
//   2. No locale carries a key `en` does not have — dead translations left
//      behind by a rename or a deleted callsite.
//   3. No locale is missing a key `en` has. A feature that ships English-only
//      into seven languages fails the build at the commit that did it, instead
//      of being found by whoever switches language months later.
//
// A fourth rule guards the other end — copy that never reached a catalog at all:
//
//   4. No notification or toast is handed a literal string. Those calls are the
//      app talking to the user, and a sentence written at the callsite is a
//      sentence the three rules above cannot see: it ships English to all eight
//      locales and no `check-locales` run will ever mention it. It had happened
//      four times, twice with English grammar built in code (a pluralizing `s`,
//      an interpolated verb) — shapes that stay wrong in every other language
//      even once someone notices.
//
// `en` is the reference: add a key there and the build names the seven files
// that still owe a translation.

import { readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";

const LOCALES_DIR = new URL("../src/lib/i18n/locales/", import.meta.url).pathname;
const SRC_DIR = new URL("../src/", import.meta.url).pathname;
const KEY_PATTERN = /^\s*"([^"]+)":/gm;
// A notify/toast call whose first argument opens a string literal.
const LITERAL_COPY_PATTERN =
  /\b(?:notifyError|notifyInfo|toast\.(?:success|error|info|warning|message))\(\s*[`"']/g;

const duplicates = [];

function keysOf(file) {
  const source = readFileSync(join(LOCALES_DIR, file), "utf8");
  const seen = new Set();
  for (const match of source.matchAll(KEY_PATTERN)) {
    if (seen.has(match[1])) duplicates.push(`${file}: "${match[1]}" declared twice`);
    seen.add(match[1]);
  }
  return seen;
}

const files = readdirSync(LOCALES_DIR).filter((name) => name.endsWith(".ts"));
const catalogs = new Map(files.map((file) => [file.replace(/\.ts$/, ""), keysOf(file)]));

const en = catalogs.get("en");
if (!en) {
  console.error("check-locales: en.ts is the reference catalog and is missing");
  process.exit(1);
}

const failures = [...duplicates];

function note(locale, keys, label) {
  if (keys.length === 0) return;
  const sample = keys.slice(0, 8).join(", ");
  const rest = keys.length > 8 ? `, … (+${keys.length - 8} more)` : "";
  failures.push(`${locale}: ${keys.length} ${label} — ${sample}${rest}`);
}

for (const [locale, keys] of catalogs) {
  if (locale === "en") continue;
  note(
    locale,
    [...keys].filter((key) => !en.has(key)),
    "key(s) absent from en",
  );
  note(
    locale,
    [...en].filter((key) => !keys.has(key)),
    "key(s) missing",
  );
}

function* sourceFiles(dir) {
  for (const entry of readdirSync(dir)) {
    const path = join(dir, entry);
    if (statSync(path).isDirectory()) {
      yield* sourceFiles(path);
    } else if (/\.tsx?$/.test(entry) && !/\.test\.tsx?$/.test(entry)) {
      yield path;
    }
  }
}

for (const path of sourceFiles(SRC_DIR)) {
  const source = readFileSync(path, "utf8");
  for (const match of source.matchAll(LITERAL_COPY_PATTERN)) {
    const line = source.slice(0, match.index).split("\n").length;
    const relative = path.slice(SRC_DIR.length);
    failures.push(`${relative}:${line}: literal copy in ${match[0].trim()} — use t("key")`);
  }
}

if (failures.length > 0) {
  console.error(`check-locales: ${failures.length} problem(s)\n`);
  for (const failure of failures) console.error(`  ${failure}`);
  process.exit(1);
}
console.log(`check-locales: ${en.size} keys, complete across ${catalogs.size} locales`);
