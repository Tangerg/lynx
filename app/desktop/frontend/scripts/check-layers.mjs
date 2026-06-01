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
// NOT enforced here (intentional, see CLAUDE.md): protocol/agui → plugins/sdk
// is the reducer's dispatcher seam (events route to host.agui.onCore
// handlers) — that's the core "kernel doesn't grow" design, not a leak.

import { execFileSync } from "node:child_process";

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
  // AG-UI reducer/viewState. MAY reach sdk (dispatcher seam) + lib + rpc;
  // must not reach UI, stores, or wiring.
  protocol: [...UI, "state", "infra", "main"],
  // The plugin SDK is a platform layer — it must not depend on the UI it
  // is consumed by (locks the MessageContext inversion fix).
  sdk: [...UI],
  // Stores are below the UI.
  state: [...UI],
  // Utility layer — no UI, no concrete plugins.
  lib: [...UI],
};

// Documented exceptions: "importer↦importee" file pairs (src-relative)
// that are knowingly allowed despite the rule. Empty today.
const ALLOWED_EDGES = new Set([]);

let raw;
try {
  raw = execFileSync("npx", ["madge", "--extensions", "ts,tsx", "--json", "src/"], {
    encoding: "utf8",
    maxBuffer: 32 * 1024 * 1024,
  });
} catch (err) {
  raw = err.stdout?.toString() ?? "";
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
  const from = layerOf(file);
  const forbidden = FORBIDDEN[from];
  if (!forbidden) continue;
  for (const dep of deps) {
    const to = layerOf(dep);
    if (forbidden.includes(to) && !ALLOWED_EDGES.has(`${file}↦${dep}`)) {
      violations.push({ file, dep, from, to });
    }
  }
}

if (violations.length > 0) {
  console.error(`[check-layers] Found ${violations.length} layer-boundary violation(s):`);
  for (const v of violations) {
    console.error(`  ${v.from} → ${v.to}:  ${v.file}  →  ${v.dep}`);
  }
  console.error("");
  console.error("An inner layer is importing an outer one. Either invert the");
  console.error("dependency, or — if genuinely intentional — add the edge to");
  console.error("ALLOWED_EDGES in scripts/check-layers.mjs with a comment.");
  process.exit(1);
}

console.log("[check-layers] OK — no layer-boundary violations.");
