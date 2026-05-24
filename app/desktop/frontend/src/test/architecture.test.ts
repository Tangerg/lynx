// Architecture conformance — enforces the Clean-Architecture-inspired
// layering by scanning import statements at test time.
//
// Why a test, not ESLint: we don't have ESLint configured yet, and a
// vitest test runs in the same toolchain as everything else — no new
// dev dependencies, no plugin authoring. The trade-off is that this
// catches violations on `pnpm test`, not on save in the editor; good
// enough for now. Migrate to `eslint-plugin-boundaries` later if the
// rule set grows.
//
// The layers Lyra recognises (see ARCHITECTURE.md "整洁架构 → Lyra 适配"):
//
//   domain/   pure types + outbound contracts (gateways). zero deps.
//   infra/    gateway implementations. depends on domain (+ lib/http).
//   main/     composition root. wires infra into gateway slots.
//   components/state/plugins/lib/ ... = presentation/cross-cutting.
//     reads domain types directly; talks to infra ONLY via main/container.
//
// If you're hitting one of these failures and think the rule is wrong,
// edit it here — but explain the reason in the relevant ARCHITECTURE.md
// section so the rule stays consistent with intent.

import { describe, it, expect } from "vitest";
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join, relative, resolve } from "node:path";

const SRC_ROOT = resolve(__dirname, "..");

function walk(dir: string, out: string[] = []): string[] {
  let entries: string[];
  try {
    entries = readdirSync(dir);
  } catch {
    return out;
  }
  for (const entry of entries) {
    const full = join(dir, entry);
    const stat = statSync(full);
    if (stat.isDirectory()) {
      walk(full, out);
    } else if (/\.(ts|tsx)$/.test(entry) && !/\.test\.tsx?$/.test(entry)) {
      out.push(full);
    }
  }
  return out;
}

// Pull every `import ... from "x"` / `import "x"` specifier out of `src`.
// Static imports only — dynamic `import(...)` is allowed in this codebase
// (plugin sideloader uses it deliberately) and isn't a layering concern.
function staticImports(src: string): string[] {
  const re = /^\s*import\s+(?:[^"';]+\s+from\s+)?["']([^"']+)["']/gm;
  const out: string[] = [];
  let m: RegExpExecArray | null;
  while ((m = re.exec(src)) !== null) out.push(m[1]);
  return out;
}

// Named import-pattern bundles. Composed per-layer below so a regex is
// only written once and the rule for each layer reads as a checklist.

const REACT_AND_LIBS = [/^react$/, /^react\//, /^zustand/, /^@ag-ui\//];
const UI_LAYERS = [/^@\/components/, /^@\/state/, /^@\/plugins/, /^@\/pages/];
const INTERNAL_NOT_DOMAIN = [
  ...UI_LAYERS,
  /^@\/infra/,
  /^@\/main/,
  /^@\/lib/,
  /^@\/protocol/,
  /^@\/utils/,
];

// Source-level patterns (not import specifiers — actual code shapes
// we forbid in certain layers).
const FORBID_FETCH_IN_DOMAIN = [/\bfetch\s*\(/, /localStorage\./, /sessionStorage\./];

type Rule = {
  layer: string;
  files: string[];
  /** A spec is forbidden if any of these regexes match. */
  forbiddenImports: RegExp[];
  /** Extra source-text checks (raw string match). Used for fetch() etc. */
  forbiddenSource?: RegExp[];
};

function assertRule(rule: Rule) {
  const violations: string[] = [];
  for (const file of rule.files) {
    const src = readFileSync(file, "utf-8");
    const rel = relative(SRC_ROOT, file);

    for (const imp of staticImports(src)) {
      for (const pattern of rule.forbiddenImports) {
        if (pattern.test(imp)) {
          violations.push(`  ${rel}: forbidden import "${imp}" (matched ${pattern})`);
        }
      }
    }
    for (const pattern of rule.forbiddenSource ?? []) {
      if (pattern.test(src)) {
        violations.push(`  ${rel}: forbidden pattern ${pattern} found in source`);
      }
    }
  }
  if (violations.length > 0) {
    throw new Error(`Architecture rule violated in ${rule.layer}:\n` + violations.join("\n"));
  }
}

describe("architecture conformance", () => {
  // ---- domain/ ----------------------------------------------------------
  // Pure types + contracts. Anything that wires to a framework, transport,
  // or store breaks the rule that domain has zero outward dependencies.
  it("domain/ contains no react / zustand / cross-layer imports", () => {
    const files = walk(resolve(SRC_ROOT, "domain"));
    expect(files.length).toBeGreaterThan(0);
    assertRule({
      layer: "domain/",
      files,
      forbiddenImports: [...REACT_AND_LIBS, ...INTERNAL_NOT_DOMAIN],
      forbiddenSource: FORBID_FETCH_IN_DOMAIN,
    });
  });

  // ---- infra/ -----------------------------------------------------------
  // Implements domain gateways using external libs / fetch. Allowed to
  // pull from @/domain and @/lib/http (the transport facade). Forbidden to
  // reach into UI, store, plugins, main, protocol.
  it("infra/ depends only on domain (+ lib/http) and never on UI/state/plugins/main", () => {
    const files = walk(resolve(SRC_ROOT, "infra"));
    expect(files.length).toBeGreaterThan(0);
    assertRule({
      layer: "infra/",
      files,
      forbiddenImports: [...UI_LAYERS, /^@\/main/, /^@\/protocol/],
    });
  });

  // ---- presentation / cross-cutting layers ------------------------------
  // components/, state/, plugins/, lib/, pages/ must not punch through to
  // infra directly — they go through main/container which holds the wired
  // gateways. This is what keeps useApprovalSubmit transport-agnostic.
  it("presentation code does not import @/infra/* directly (must go via main/container)", () => {
    const layers = ["components", "state", "plugins", "lib", "pages"];
    const files = layers.flatMap((l) => walk(resolve(SRC_ROOT, l)));
    expect(files.length).toBeGreaterThan(0);
    assertRule({
      layer: "presentation",
      files,
      forbiddenImports: [/^@\/infra/],
    });
  });
});
