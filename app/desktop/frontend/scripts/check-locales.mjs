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
// `en` is the reference: add a key there and the build names the seven files
// that still owe a translation.

import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";

const LOCALES_DIR = new URL("../src/lib/i18n/locales/", import.meta.url).pathname;
const KEY_PATTERN = /^\s*"([^"]+)":/gm;

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

if (failures.length > 0) {
  console.error(`check-locales: ${failures.length} problem(s)\n`);
  for (const failure of failures) console.error(`  ${failure}`);
  process.exit(1);
}
console.log(`check-locales: ${en.size} keys, complete across ${catalogs.size} locales`);
