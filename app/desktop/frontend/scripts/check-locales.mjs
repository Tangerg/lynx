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
//      of the chat stream in English.
//  10. No SENTENCE in `application/`. This ring was left out for a while because
//      it does carry developer strings the rule would trip on; the cost of that
//      gap was four sibling view models each inventing English for the same
//      header slot ("2 MCP active · 5 configured", "3 commands", "7 matches",
//      "1 unread · 4 total") plus a task pill title, none of which any catalog
//      could see. Both families it was waiting on are now handled precisely
//      rather than by an exemption list: a port's not-configured message is read
//      out of the `createSingletonPort` argument and dropped, and a sentence is
//      required to LOOK like one — a space plus a word that starts lowercase —
//      so a curated list of font families ("SF Pro Text") isn't one.
//  11. No copy inside a component. A view is the right ring for words, but a
//      literal there still bypasses all eight catalogs, and twenty-six had: five
//      tool previews' placeholders, four overflow footers, a diagnostics panel's
//      title / description / signal switch, the plugin boundary's error line, and
//      the loader's screen-reader label. Three shapes are checked —
//      a text node of two or more words, a text node that mixes an expression
//      with words ("{count} more"), and a string-valued prop that reads as a
//      sentence. Single tokens are NOT caught: a regex can't tell the `esc` on a
//      keycap or a `json` badge from a word, and pretending otherwise would make
//      the rule noisy enough to be turned off.
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
//   9. No key the tree never names. Rules 2 and 3 keep the catalogs equal to each
//      other and rule 5 keeps the code inside them, but nothing looked the other
//      way: 57 keys outlived the callsites that used them (a rewritten welcome
//      screen, a replaced tab menu, a diagnostics table that now prints OTel's own
//      field names) and sat there translated eight times over. A key is "named" by
//      any string literal in the tree — `t("x")`, or a spec field carrying it —
//      or by a template prefix for the tables built as `` t(`x.${y}`) ``.
//
// And one on the values, not the keys:
//
//   7. A translated value carries exactly the `{{placeholders}}` its English
//      counterpart does. This is the one translation defect a reviewer who
//      doesn't read the language can still catch, and it's the most damaging:
//      a dropped `{{count}}` renders a sentence with the number silently gone.
//      (The 1265 non-English strings are unreviewed by native speakers — that
//      part no build can check. This checks what it can.)
//  12. No value writes "..." where it means "…". Three periods is a different
//      glyph at a different width, so it does not line up with the real one in
//      the row above it. The catalogs had settled on `…` in 43 strings and one
//      search placeholder on "..." — in all eight locales, because whoever wrote
//      it copied the row. The typography is the smaller half of the argument:
//      the convention already existed and one string sat outside it unseen,
//      which is the shape every other rule here guards.
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
// Use cases: they may take a translator, but they may not know one locale's words.
const USE_CASE_RING = /plugins\/builtin\/.+\/application\/.+\.tsx?$/;
// A port's "… is not configured" is thrown at a developer before any screen exists.
const PORT_MESSAGE = /createSingletonPort(?:<[^>]*>)?\(\s*(["'`])(?:\\.|(?!\1)[\s\S])*?\1/g;
// `${ … }` inside a template, tolerating one level of nesting (`t("k", { n })`).
const INTERPOLATION = /\$\{(?:[^{}]|\{[^{}]*\})*\}/g;
// A JSX text node with no expression in it: `>Some words</`. The `>` must close
// a tag, so it may not be preceded by a space — `count > 0 ? … : …` inside an
// expression is a comparison, not the end of `<div>`.
const JSX_TEXT = /[^\s]>([^<>{}]{4,}?)<\//g;
// Sentence-case words on their own: "Rename", "Pin to top". A menu item's verb is
// one token, which the two-word test below waves through — that is how five
// English labels sat in a session's context menu in an otherwise localised app.
// Identifiers escape it: they are lowercase, ALL-CAPS, or carry punctuation.
const TITLE_CASE_COPY = /^[A-Z][a-z]{2,}(?: [A-Za-z]+){0,4}$/;
// A JSX child expression that holds a quoted string: `>{fav ? "Unpin" : "Pin"}<`.
// Children hold variables, so a quoted word here is copy the component authored.
// The inner text may hold no braces (a nested expression) and no angle brackets —
// either would mean the match ran past this node into another element. An
// attribute cannot match: its `{` follows `=`, and this one follows `>`. Unlike the
// text rule above, the `>` may be preceded by a newline, because prettier puts a
// multi-line tag's `>` on its own line — which is exactly where the five English
// labels in a session's context menu were hiding.
const JSX_CHILD_EXPRESSION = />\s*\{([^{}<>]*(?:"[^"\n]*"|'[^'\n]*')[^{}<>]*)\}\s*<\//g;
const QUOTED_WORDS = /"([^"\n]+)"|'([^'\n]+)'/g;
// A JSX text node that mixes expressions with literal text: `>… {count} more</`.
const JSX_MIXED = /[^\s]>([^<>]*\{[^<>]*\}[^<>]*)<\//g;
// `&nbsp;` and friends are typesetting, not words.
const HTML_ENTITY = /&[a-zA-Z]+;|&#\d+;/g;
// A string-valued JSX prop. `className` and friends carry mechanisms, not words.
const JSX_PROP = /\s([a-zA-Z][\w-]*)=["]([^"]*[ ][^"]*)["]/g;
// Any `*ClassName` is a class list the component forwards onto an element it
// renders — `scrollClassName` onto a scroll library's node, `contentClassName` onto
// a disclosure's panel. Naming them one at a time meant the guard called the next
// one copy, which is a false positive that reads as "this needs translating".
const MECHANISM_PROP =
  /^(?:[a-z][\w]*ClassName|className|class|style|d|viewBox|accept|srcSet|sizes|content|rel)$/;

/**
 * Does this read as a sentence rather than as data?
 *
 * A space plus a word that starts lowercase. Curated identifier lists live in
 * `application/` for good reasons — the font families the picker offers, an MCP
 * server's own name — and "SF Pro Text" is not copy. "Recent tasks" is.
 */
function looksLikeSentence(text) {
  return / /.test(text) && /(?:^|[^A-Za-z])[a-z]{3,}/.test(text);
}

/** The literal halves of a template literal — what a reader actually sees. */
function literalParts(raw) {
  return raw.split(INTERPOLATION);
}

/** Text at brace depth 0 — the words in a mixed JSX node, minus the expressions. */
function textOutsideExpressions(node) {
  let depth = 0;
  let out = "";
  for (const char of node) {
    if (char === "{") depth++;
    else if (char === "}") depth = Math.max(0, depth - 1);
    else if (depth === 0) out += char;
  }
  return out;
}

// A spec whose label the shell resolves with t(): the factory's return type says
// so. Its label/title must therefore BE a key — `label: "Shortcuts"` renders as
// itself, which reads as English hardcoded into a localised rail.
const KEYED_SPEC_FACTORY = /\)\s*:\s*(?:SettingsPaneSpec|WorkspaceViewSpec|CommandSpec)\s*\{/g;
const SPEC_COPY_FIELD = /\b(label|title|description):\s*"([^"]+)"/g;
const CATALOG_KEY = /^[a-z][\w-]*(?:\.[\w-]+)+$/;

// A registration call — its argument is a spec that outlives this moment.
const CONTRIBUTION_CALL =
  /\b(?:extensions\.contribute|commands\.register|shortcuts\.register|registerSettingsPane)\s*\(/g;
const RESOLVED_COPY = /(?<![\w.])t\(\s*["'`]/;

// `` t(`rpcError.${type}`) `` — the static half covers every key beneath it.
const DYNAMIC_KEY_PREFIX = /`([a-zA-Z][\w.]*\.)\$\{/g;

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

// Rule 12 — one character, not three periods. See the header for why it earns a gate.
for (const [locale] of catalogs) {
  for (const [key, value] of valuesOf(`${locale}.ts`)) {
    if (value.includes("...")) failures.push(`${locale}: "${key}" writes "..." — use "…"`);
  }
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

  // Rule 10 — a use case may hold a rule about copy, never the copy.
  if (USE_CASE_RING.test(relative)) {
    for (const line of code.replace(PORT_MESSAGE, "createSingletonPort(").split("\n")) {
      // A `when:` clause is an expression in the command DSL, not a sentence.
      if (line.includes("console.") || line.includes("Error(") || /\bwhen:/.test(line)) continue;
      for (const match of line.matchAll(PROSE_PATTERN)) {
        for (const part of literalParts(match[1] ?? match[2] ?? "")) {
          if (looksLikeSentence(part)) {
            failures.push(
              `${relative}: a use case wrote copy — "${part.trim()}" belongs in a catalog, resolved by the view (or take a Translate)`,
            );
          }
        }
      }
    }
  }

  // Rule 11 — a component renders copy; it doesn't author it.
  if (relative.endsWith(".tsx")) {
    for (const match of code.matchAll(JSX_TEXT)) {
      const text = match[1].replace(/\s+/g, " ").trim();
      if (TWO_WORDS.test(text) || TITLE_CASE_COPY.test(text)) {
        failures.push(`${relative}: literal copy in a component — "${text}" belongs in a catalog`);
      }
    }
    for (const match of code.matchAll(JSX_CHILD_EXPRESSION)) {
      for (const quoted of match[1].matchAll(QUOTED_WORDS)) {
        const text = (quoted[1] ?? quoted[2] ?? "").trim();
        if (TWO_WORDS.test(text) || TITLE_CASE_COPY.test(text)) {
          failures.push(
            `${relative}: literal copy in a rendered expression — "${text}" belongs in a catalog`,
          );
        }
      }
    }
    for (const match of code.matchAll(JSX_MIXED)) {
      const text = textOutsideExpressions(match[1])
        .replace(HTML_ENTITY, " ")
        .replace(/\s+/g, " ")
        .trim();
      if (/[A-Za-z]{3,}/.test(text)) {
        failures.push(
          `${relative}: literal copy beside an expression — "${text}" belongs in a catalog`,
        );
      }
    }
    for (const match of code.matchAll(JSX_PROP)) {
      const [, prop, value] = match;
      if (MECHANISM_PROP.test(prop)) continue;
      if (looksLikeSentence(value)) {
        failures.push(`${relative}: ${prop}="${value}" is copy — pass a catalog key or a t() call`);
      }
    }
  }
}

// Rule 12 — a spec the shell resolves with t() carries keys, not words.
{
  for (const path of sourceFiles(SRC_DIR)) {
    const relative = path.slice(SRC_DIR.length);
    if (relative.endsWith(".test.ts") || relative.endsWith(".test.tsx")) continue;
    const code = withoutComments(readFileSync(path, "utf8"));
    for (const factory of code.matchAll(KEYED_SPEC_FACTORY)) {
      const body = code.slice(factory.index, code.indexOf("\n}", factory.index));
      for (const field of body.matchAll(SPEC_COPY_FIELD)) {
        const [, name, value] = field;
        if (CATALOG_KEY.test(value)) continue;
        failures.push(
          `${relative}: ${name}: "${value}" is copy in a spec the shell resolves — use a catalog key`,
        );
      }
    }
  }
}

// Rule 9 — every key is named somewhere in the tree.
{
  const named = [];
  const prefixes = new Set();
  for (const path of sourceFiles(SRC_DIR)) {
    const relative = path.slice(SRC_DIR.length);
    if (relative.startsWith("lib/i18n/locales/")) continue;
    const source = readFileSync(path, "utf8");
    named.push(source);
    for (const match of source.matchAll(DYNAMIC_KEY_PREFIX)) prefixes.add(match[1]);
  }
  const blob = named.join("\n");
  const unnamed = [...en].filter(
    (key) =>
      !blob.includes(`"${key}"`) &&
      !blob.includes(`'${key}'`) &&
      ![...prefixes].some((prefix) => key.startsWith(prefix)),
  );
  if (unnamed.length > 0) {
    const sample = unnamed.slice(0, 8).join(", ");
    const rest = unnamed.length > 8 ? `, … (+${unnamed.length - 8} more)` : "";
    failures.push(`${unnamed.length} key(s) nothing names — delete them: ${sample}${rest}`);
  }
}

if (failures.length > 0) {
  console.error(`check-locales: ${failures.length} problem(s)\n`);
  for (const failure of failures) console.error(`  ${failure}`);
  process.exit(1);
}
console.log(`check-locales: ${en.size} keys, complete across ${catalogs.size} locales`);
