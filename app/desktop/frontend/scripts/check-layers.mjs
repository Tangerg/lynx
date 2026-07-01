#!/usr/bin/env node
// Layer-boundary guard. Complements check-circular.mjs: that one forbids
// cycles, this one forbids *upward* / cross-layer import edges that the
// clean-architecture layering disallows. Run off the same madge graph.
//
// Philosophy mirrors CLAUDE.md's "强反向不变量 (known wrong directions)":
// rather than a full allow-matrix (brittle, false-positive prone), each
// guarded layer declares the set of layers it must NEVER import. Edges to
// any other layer are allowed — so this catches the architectural
// regressions we care about (UI/plugin upward deps, domain/infra/rpc
// purity) without policing every legitimate inward dependency.
//
// NOT enforced here (intentional, see CLAUDE.md): protocol → plugins/sdk is
// the reducer's dispatcher seam (StreamEvents route to host.events.onStream
// handlers) — that's the core "kernel doesn't grow" design, not a leak.

import { execFileSync } from "node:child_process";
import { closeSync, openSync, readFileSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

// Ordered longest-prefix-first: first match wins. Paths are relative to
// src/ (how madge reports them when invoked with `src/`).
const LAYER_PREFIXES = [
  ["plugins/sdk/", "sdk"],
  ["plugins/builtin/", "builtin"],
  ["plugins/", "plugins-glue"], // Slot / PluginProvider / etc. — UI glue
  ["protocol/", "protocol"],
  ["domain/", "domain"],
  ["infra/", "infra"],
  ["main/", "main"],
  ["rpc/", "rpc"],
  ["state/", "state"],
  ["lib/", "lib"],
  ["components/", "components"],
  ["pages/", "pages"],
];

function layerOf(path) {
  for (const [prefix, layer] of LAYER_PREFIXES) if (path.startsWith(prefix)) return layer;
  return "other"; // assets / styles / test helpers / bare entry — unguarded
}

// Per guarded layer: the layers it must NEVER import. The inner three
// (domain/infra/rpc) are near-total: they're meant to be self-contained
// or contract-only. The outer guards lock the upward edges.
const UI = ["components", "pages", "builtin", "plugins-glue"];
const FORBIDDEN = {
  // NOTE: `domain/` + `infra/` hold NO files today — the early clean-arch
  // gateway seam was superseded by the `rpc/` layer + main/container.ts
  // (ARCHITECTURE.md §3.1). Kept as reserved guards: if those dirs ever come
  // back they're locked to contract-/impl-only purity from the first file.
  // Pure contracts — import nothing else in src.
  domain: [
    "infra",
    "main",
    "rpc",
    "protocol",
    "state",
    "sdk",
    "builtin",
    "plugins-glue",
    "lib",
    "components",
    "pages",
  ],
  // Gateway impls — only the domain contracts they implement.
  infra: [
    "main",
    "rpc",
    "protocol",
    "state",
    "sdk",
    "builtin",
    "plugins-glue",
    "lib",
    "components",
    "pages",
  ],
  // Standalone protocol layer — externals + its own files only.
  rpc: [
    "domain",
    "infra",
    "main",
    "protocol",
    "state",
    "sdk",
    "builtin",
    "plugins-glue",
    "lib",
    "components",
    "pages",
  ],
  // Protocol reducer/viewState. MAY reach sdk (dispatcher seam) + lib + rpc;
  // must not reach UI, stores, or wiring.
  protocol: [...UI, "state", "infra", "main"],
  // The plugin SDK is a platform layer — it must not depend on the UI it
  // is consumed by (locks the MessageContext inversion fix).
  sdk: [...UI],
  // Stores are below the UI.
  state: [...UI],
  // Utility layer — no UI, no concrete plugins.
  lib: [...UI],
  // The view layer reaches the backend only through hooks / stores (state,
  // lib/data query hooks, plugin SDK selectors) — never the composition root
  // (`main/container`) or the raw protocol client (`rpc`) directly. Keeps
  // components a thin presentation + store-wiring layer; business access stays
  // behind a minimal hook/selector seam.
  components: ["main", "rpc"],
  pages: ["main", "rpc"],
};

// Documented exceptions: "importer↦importee" file pairs (src-relative)
// that are knowingly allowed despite the rule. Empty today.
const ALLOWED_EDGES = new Set([]);

const CONTEXT_INTERNAL_RE =
  /^plugins\/builtin\/([^/]+)\/(?:application|presentation|domain|adapters)\//;

function builtinContext(path) {
  const match = /^plugins\/builtin\/([^/]+)\//.exec(path);
  return match?.[1];
}

function contextInternalViolation(file, dep) {
  const match = CONTEXT_INTERNAL_RE.exec(dep);
  if (!match) return null;
  const depContext = match[1];
  const fromContext = builtinContext(file);
  if (fromContext === depContext) return null;
  return { file, dep, from: fromContext ?? "outside-builtin", to: depContext };
}

// Redirect madge's JSON to a temp FILE rather than capturing its stdout pipe.
// madge calls process.exit() before an async stdout *pipe* finishes draining
// (Node's classic exit-truncates-piped-stdout bug), so a captured pipe is
// silently capped at the 64KB buffer once the graph grows past it — which
// check:circular dodges only because `--circular` output is tiny. A file fd
// flushes synchronously on close, so the whole graph survives at any size.
const graphFile = join(tmpdir(), "lyra-check-layers-madge.json");
let raw = "";
try {
  const fd = openSync(graphFile, "w");
  try {
    execFileSync(
      "npx",
      ["madge", "--extensions", "ts,tsx", "--ts-config", "tsconfig.json", "--json", "src/"],
      { stdio: ["ignore", fd, "inherit"] },
    );
  } catch {
    // madge can exit non-zero on warnings yet still write a full graph — read
    // whatever landed and let JSON.parse below be the judge.
  } finally {
    closeSync(fd);
  }
  raw = readFileSync(graphFile, "utf8");
} finally {
  rmSync(graphFile, { force: true });
}

let graph;
try {
  graph = JSON.parse(raw);
} catch {
  console.error("[check-layers] madge did not produce valid JSON:");
  console.error(raw);
  process.exit(2);
}

const violations = [];
for (const [file, deps] of Object.entries(graph)) {
  // Tests may import across layers to wire fixtures (e.g. loading a plugin to
  // exercise the reducer). The layering invariant is about production
  // dependency direction, so skip test files as importers.
  if (/\.(test|spec)\.[tj]sx?$/.test(file)) continue;
  const from = layerOf(file);
  const forbidden = FORBIDDEN[from];
  if (!forbidden) continue;
  for (const dep of deps) {
    const to = layerOf(dep);
    if (forbidden.includes(to) && !ALLOWED_EDGES.has(`${file}↦${dep}`)) {
      violations.push({ file, dep, from, to });
    }
    const contextViolation = contextInternalViolation(file, dep);
    if (contextViolation && !ALLOWED_EDGES.has(`${file}↦${dep}`)) {
      violations.push({
        file,
        dep,
        from: `context:${contextViolation.from}`,
        to: `context-internal:${contextViolation.to}`,
      });
    }
  }
}

if (violations.length > 0) {
  console.error(`[check-layers] Found ${violations.length} layer-boundary violation(s):`);
  for (const v of violations) {
    console.error(`  ${v.from} → ${v.to}:  ${v.file}  →  ${v.dep}`);
  }
  console.error("");
  console.error("An inner layer is importing an outer one, or a plugin context");
  console.error("is reaching into another context's internals. Either invert");
  console.error("the dependency / use a public surface, or — if genuinely");
  console.error("intentional — add the edge to ALLOWED_EDGES with a comment.");
  process.exit(1);
}

console.log("[check-layers] OK — no layer-boundary violations.");
