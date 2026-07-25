#!/usr/bin/env node
// Locale-catalog guard.
//
// Why this exists: the catalogs are typed `Record<string, string>`, so a locale
// that never learned a key is not a compile error — it is a silent fall back to
// English at runtime. Nothing was watching, and the gap grew to a third of the
// app for five languages. This makes the number visible on every run and stops it
// growing.
//
// Two rules:
//
//   1. HARD — a locale may not carry a key `en` does not have. Those are dead
//      translations: the callsite is gone, or the key was renamed on one side.
//      Enforced at zero, because it is at zero.
//
//   2. RATCHET — the count of keys a locale is MISSING may not exceed the
//      baseline below. It cannot be zero today without ~1800 authored
//      translations, and inventing those unreviewed would be worse than the gap.
//      The baseline is debt, recorded rather than hidden: lower a number when you
//      translate, and the guard holds the new floor. Never raise one — a raise
//      means a feature shipped English-only into seven languages.

import { readFileSync, readdirSync } from "node:fs";
import { join } from "node:path";

const LOCALES_DIR = new URL("../src/lib/i18n/locales/", import.meta.url).pathname;

// Keys each locale still owes `en`. See rule 2 — these may only shrink.
const MISSING_BASELINE = {
  de: 253,
  es: 253,
  fr: 253,
  ja: 253,
  ko: 253,
  "zh-TW": 235,
  zh: 70,
};

const KEY_PATTERN = /^\s*"([^"]+)":/gm;

function keysOf(file) {
  const source = readFileSync(join(LOCALES_DIR, file), "utf8");
  return new Set(Array.from(source.matchAll(KEY_PATTERN), (match) => match[1]));
}

const files = readdirSync(LOCALES_DIR).filter((name) => name.endsWith(".ts"));
const catalogs = new Map(files.map((file) => [file.replace(/\.ts$/, ""), keysOf(file)]));

const en = catalogs.get("en");
if (!en) {
  console.error("check-locales: en.ts is the reference catalog and is missing");
  process.exit(1);
}

const failures = [];
const report = [];

for (const [locale, keys] of catalogs) {
  if (locale === "en") continue;

  const extra = [...keys].filter((key) => !en.has(key));
  if (extra.length > 0) {
    failures.push(
      `${locale}: ${extra.length} key(s) absent from en — ${extra.slice(0, 5).join(", ")}`,
    );
  }

  const missing = [...en].filter((key) => !keys.has(key)).length;
  const baseline = MISSING_BASELINE[locale];
  if (baseline === undefined) {
    failures.push(`${locale}: no baseline recorded — add one to MISSING_BASELINE`);
  } else if (missing > baseline) {
    failures.push(
      `${locale}: ${missing} missing, baseline ${baseline} — a new key landed in en only`,
    );
  } else {
    if (missing < baseline) {
      failures.push(`${locale}: ${missing} missing, baseline still says ${baseline} — lower it`);
    }
    report.push(`${locale} ${missing}/${en.size}`);
  }
}

if (failures.length > 0) {
  console.error(`check-locales: ${failures.length} problem(s)\n`);
  for (const failure of failures) console.error(`  ${failure}`);
  process.exit(1);
}
console.log(`check-locales: ${en.size} keys in en; untranslated — ${report.join(", ")}`);
