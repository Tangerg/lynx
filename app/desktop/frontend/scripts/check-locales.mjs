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
// Two more close the loop in the other direction — code → catalog:
//
//   5. Every `t("literal")` in the tree names a key `en` has. The three rules
//      above compare catalogs against each other and never look at the code, so
//      a key that exists nowhere renders as its own name ("runError.unknown")
//      and no run says a word. Dynamic keys (a table of them, `t(`x.${y}`)`)
//      can't be checked here and aren't.
//   6. No copy in `presentation/` or `domain/`. Those rings map a model into a
//      view model or hold a rule; the words belong to the view, which has a
//      translator. Five modules had drifted — tool labels, meta chips, a group
//      summary, nine danger reasons, a run digest — so seven locales read parts
//      of the chat stream in English. `application/` is NOT covered: it
//      legitimately carries developer strings (port messages, log lines), and a
//      rule that needs an exemption list per callsite is a rule that stops
//      being read.
//
// And one on WHEN copy is resolved:
//
//   8. A contributed spec carries catalog keys, not resolved text. `setLocale`
//      changes the language and nothing re-registers contributions, so a
//      `t(...)` evaluated while building a spec freezes that label in the locale
//      the app booted in — while the specs beside it that carry keys update
//      immediately. Twenty-six labels had been frozen this way (commands,
//      shortcut descriptions, message roles, a tool action, one settings pane out
//      of twelve). Two shapes are checked: a `t(` inside a contribution call, and
//      a `t(` anywhere in a `*Contributions.ts` spec factory.
//
// And one on the values, not the keys:
//
//   7. A translated value carries exactly the `{{placeholders}}` its English
//      counterpart does. This is the one translation defect a reviewer who
//      doesn't read the language can still catch, and it's the most damaging:
//      a dropped `{{count}}` renders a sentence with the number silently gone.
//      (The 1265 non-English strings are unreviewed by native speakers — that
//      part no build can check. This checks what it can.)
//
// `en` is the reference: add a key there and the build names the seven files
// that still owe a translation.

import { readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";

const LOCALES_DIR = new URL("../src/lib/i18n/locales/", import.meta.url).pathname;
const SRC_DIR = new URL("../src/", import.meta.url).pathname;
const KEY_PATTERN = /^\s*"([^"]+)":/gm;
// key → value, both quote styles (a value holding a double quote is single-quoted)
// and prettier's wrapped form, where the value sits on the next line.
const PAIR_PATTERN = /"([^"]+)":\s*(?:"((?:[^"\\]|\\.)*)"|'((?:[^'\\]|\\.)*)')/g;
const PLACEHOLDER_PATTERN = /\{\{(\w+)\}\}/g;
// A notify/toast call whose first argument opens a string literal.
const LITERAL_COPY_PATTERN =
  /\b(?:notifyError|notifyInfo|toast\.(?:success|error|info|warning|message))\(\s*[`"']/g;
// `t("some.key")` — only literal keys can be checked against the catalog.
const LITERAL_KEY_PATTERN = /\bt\(\s*"([^"]+)"/g;
// A string or template holding two consecutive words: prose, not an identifier.
const PROSE_PATTERN = /(?:"([^"\n]*)"|`([^`\n]*)`)/g;
const TWO_WORDS = /[A-Za-z]{2,}[ ,]+[A-Za-z]{2,}/;
// Rings whose job is a model, a rule or a view model — never one locale's words.
const COPY_FREE_RING = /plugins\/builtin\/.+\/(?:presentation|domain)\/.+\.tsx?$/;

// A registration call — its argument is a spec that outlives this moment.
const CONTRIBUTION_CALL =
  /\b(?:extensions\.contribute|commands\.register|shortcuts\.register|registerSettingsPane)\s*\(/g;
const RESOLVED_COPY = /(?<![\w.])t\(\s*["'`]/;

function withoutComments(source) {
  return source.replace(/\/\*[\s\S]*?\*\//g, "").replace(/\/\/[^\n]*/g, "");
}

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

function valuesOf(file) {
  const source = readFileSync(join(LOCALES_DIR, file), "utf8");
  const out = new Map();
  for (const match of source.matchAll(PAIR_PATTERN)) {
    out.set(match[1], match[2] ?? match[3] ?? "");
  }
  return out;
}

function placeholders(value) {
  return [...value.matchAll(PLACEHOLDER_PATTERN)].map((match) => match[1]).sort();
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

const enValues = valuesOf("en.ts");
for (const [locale] of catalogs) {
  if (locale === "en") continue;
  const values = valuesOf(`${locale}.ts`);
  for (const [key, english] of enValues) {
    const translated = values.get(key);
    if (translated === undefined) continue; // rule 3 already reports the gap
    const want = placeholders(english).join(",");
    const got = placeholders(translated).join(",");
    if (want !== got) {
      failures.push(
        `${locale}: "${key}" carries {{${got || "none"}}} where en carries {{${want || "none"}}}`,
      );
    }
  }
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
  const relative = path.slice(SRC_DIR.length);
  if (relative.startsWith("lib/i18n/locales/")) continue;
  const lineOf = (index) => source.slice(0, index).split("\n").length;

  for (const match of source.matchAll(LITERAL_COPY_PATTERN)) {
    failures.push(
      `${relative}:${lineOf(match.index)}: literal copy in ${match[0].trim()} — use t("key")`,
    );
  }

  const code = withoutComments(source);
  for (const match of code.matchAll(LITERAL_KEY_PATTERN)) {
    if (!en.has(match[1])) {
      failures.push(`${relative}: t("${match[1]}") names a key no catalog has`);
    }
  }

  // Rule 8a — a spec built inline at its registration call.
  for (const match of code.matchAll(CONTRIBUTION_CALL)) {
    let depth = 0;
    let index = match.index + match[0].length - 1;
    for (; index < code.length; index += 1) {
      if (code[index] === "(") depth += 1;
      else if (code[index] === ")") {
        depth -= 1;
        if (depth === 0) break;
      }
    }
    if (RESOLVED_COPY.test(code.slice(match.index, index))) {
      failures.push(
        `${relative}:${lineOf(match.index)}: a contribution resolves copy at registration — ` +
          `carry the key, resolve it where it renders`,
      );
    }
  }

  // Rule 8b — the spec factories, by convention.
  if (/Contributions\.ts$/.test(relative) && RESOLVED_COPY.test(code)) {
    failures.push(
      `${relative}: a spec factory resolves copy — carry the key, resolve it where it renders`,
    );
  }

  if (COPY_FREE_RING.test(relative)) {
    for (const line of code.split("\n")) {
      if (line.includes("console.") || line.includes("Error(")) continue;
      for (const match of line.matchAll(PROSE_PATTERN)) {
        const text = match[1] ?? match[2] ?? "";
        if (TWO_WORDS.test(text)) {
          failures.push(`${relative}: prose in a copy-free ring — "${text}" belongs in a catalog`);
        }
      }
    }
  }
}

if (failures.length > 0) {
  console.error(`check-locales: ${failures.length} problem(s)\n`);
  for (const failure of failures) console.error(`  ${failure}`);
  process.exit(1);
}
console.log(`check-locales: ${en.size} keys, complete across ${catalogs.size} locales`);
