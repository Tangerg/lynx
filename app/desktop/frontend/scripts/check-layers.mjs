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

// A directory named any of these under plugins/builtin/<ctx>/ marks <ctx> as a
// bounded context (it has opted into the layout). `public/` is the only surface
// a foreign context may import; the rest are context-private. Contexts with no
// boundary dir (flat plugin folders like theme/ or defaults/) aren't policed.
const CONTEXT_BOUNDARY = new Set([
  "application",
  "presentation",
  "domain",
  "adapters",
  "public",
  "ui",
]);

function contextRootFromBoundary(path) {
  const parts = path.split("/");
  if (parts[0] !== "plugins" || parts[1] !== "builtin") return null;
  for (let i = 2; i < parts.length; i++) {
    if (CONTEXT_BOUNDARY.has(parts[i])) return i > 2 ? parts.slice(0, i).join("/") : null;
  }
  return null;
}

function contextRootsOf(graph) {
  const roots = new Set();
  for (const [file, deps] of Object.entries(graph)) {
    const fileRoot = contextRootFromBoundary(file);
    if (fileRoot) roots.add(fileRoot);
    for (const dep of deps) {
      const depRoot = contextRootFromBoundary(dep);
      if (depRoot) roots.add(depRoot);
    }
  }
  return [...roots].sort((a, b) => b.length - a.length);
}

function builtinContext(path, contextRoots) {
  if (!path.startsWith("plugins/builtin/")) return null;
  return contextRoots.find((root) => path === root || path.startsWith(`${root}/`)) ?? null;
}

// The builtin manifest is the plugin composition root: it imports every
// plugin's registration entry wherever it lives in the tree (a context holds
// several plugins, each with its own index/bootstrap), exactly as
// main/container may reach anything. It's exempt as an importer; peer contexts
// get no such license.
const BUILTIN_MANIFEST = "plugins/builtin/index.ts";

// A peer context may import only another context's `public/` facade. Any other
// cross-context import — including into a loose file sitting at the context
// root, not just the named-internal dirs — reaches a private part of the
// context and is a violation. This is strictly stronger than the old "must not
// reach application/domain/adapters/presentation/ui" rule: it also closes the
// loophole of importing a root-level file that lives in no boundary dir at all.
function crossContextViolation(file, dep, contextRoots) {
  if (file === BUILTIN_MANIFEST) return null; // plugin composition root
  const depContext = builtinContext(dep, contextRoots);
  if (!depContext) return null; // dep isn't inside any recognized context
  const fromContext = builtinContext(file, contextRoots);
  if (fromContext === depContext) return null; // same context — its own business
  if (dep.startsWith(`${depContext}/public/`)) return null; // the published facade
  return {
    file,
    dep,
    from: fromContext ? fromContext.replace("plugins/builtin/", "") : "outside-context",
    to: depContext.replace("plugins/builtin/", ""),
  };
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
const contextRoots = contextRootsOf(graph);
for (const [file, deps] of Object.entries(graph)) {
  // Tests may import across layers to wire fixtures (e.g. loading a plugin to
  // exercise the reducer). The layering invariant is about production
  // dependency direction, so skip test files as importers.
  if (/\.(test|spec)\.[tj]sx?$/.test(file)) continue;
  const from = layerOf(file);
  const forbidden = FORBIDDEN[from] ?? [];
  for (const dep of deps) {
    const to = layerOf(dep);
    if (forbidden.includes(to) && !ALLOWED_EDGES.has(`${file}↦${dep}`)) {
      violations.push({ file, dep, from, to });
    }
    const contextViolation = crossContextViolation(file, dep, contextRoots);
    if (contextViolation && !ALLOWED_EDGES.has(`${file}↦${dep}`)) {
      violations.push({
        file,
        dep,
        from: `context:${contextViolation.from}`,
        to: `context-private:${contextViolation.to}`,
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
  console.error("is reaching past another context's public/ facade (that facade is");
  console.error("the only surface importable across contexts). Either invert the");
  console.error("dependency / route through the public surface, or — if genuinely");
  console.error("intentional — add the edge to ALLOWED_EDGES with a comment.");
  process.exit(1);
}

console.log("[check-layers] OK — no layer-boundary violations.");
