// ESLint v9 flat config — built on @antfu/eslint-config.
//
// Antfu's base gives us: TS + React + import sort + unicorn + perfectionist
// + JSON/YAML/Markdown lint. We turn `stylistic: false` off because Prettier
// owns formatting in this repo (see .prettierrc.json) — Antfu's style rules
// would fight it on quote / semi / indent.
//
// Project-specific overrides at the bottom mute rules that produce more
// false positives than real bugs in this codebase.

import antfu from "@antfu/eslint-config";

export default antfu({
  // Framework + lang flags.
  react: true,
  typescript: true,
  vue: false,

  // Format: Prettier handles it. Drop Antfu's style rules so they don't
  // fight prettier --check.
  stylistic: false,

  ignores: [
    "dist/**",
    "wailsjs/**",
    "src/lib/shiki.ts",
  ],
}, {
  // Project-specific overrides — kept tight. Each rule muted here has a
  // reason in the comment above it.
  rules: {
    // Lyra exports plugin spec via `export default definePlugin({...})`
    // pervasively — Antfu's `import/no-default-export` would scream on
    // every plugin file.
    "import/no-default-export": "off",

    // Files in this repo routinely export a component PLUS its props
    // type or helper. The dev-server Fast Refresh warning is noise.
    "react-refresh/only-export-components": "off",

    // React 19 hook rules false-positive on legitimate ref-init-in-render
    // and useMemo'd factories. Antfu surfaces them under the `react/`
    // prefix (via @eslint-react/eslint-plugin). Real hazards are still
    // caught by the long-standing rules-of-hooks + exhaustive-deps
    // checks.
    "react/purity": "off",
    "react/static-components": "off",
    "react/set-state-in-effect": "off",

    // We use stable indices as keys for static lists deliberately. Where
    // it matters (re-orderable / removable lists), code uses stable ids.
    "react/no-array-index-key": "off",

    // Shiki and Mermaid output HTML strings — there's no React tree to
    // build from them. dangerouslySetInnerHTML is the intended path.
    "react/dom-no-dangerously-set-innerhtml": "off",

    // Plugin convention: `useAttachments` is read like a hook (called
    // inside a component) but doesn't itself call hooks. Treating it
    // like a hook keeps the SDK shape consistent.
    "react/no-unnecessary-use-prefix": "off",

    // Unused vars — warn (not error), allow `_`-prefix opt-out. Antfu
    // surfaces typescript-eslint rules under the `ts/` prefix.
    "ts/no-unused-vars": [
      "warn",
      { argsIgnorePattern: "^_", varsIgnorePattern: "^_" },
    ],
    "no-unused-vars": "off",

    // `any` is a warning, not an error — sometimes the right tool at
    // protocol boundaries (AG-UI custom event payloads, plugin spec
    // erasure).
    "ts/no-explicit-any": "warn",

    // We deliberately alias `this` to a local in storage.ts so the
    // returned object can call its own `set` from within a migration
    // step. Rare pattern; the const-binding makes intent explicit.
    "ts/no-this-alias": "off",

    // Forward-references between consts / fns inside the same file are
    // fine when both are evaluated lazily (JSX-inlined className constants,
    // declared placeholder helpers next to their public selectors, etc.).
    // We rely on TS's compile-time TDZ checks to catch the real hazards.
    "ts/no-use-before-define": "off",
    "no-use-before-define": "off",

    // Antfu defaults to forbidding `console.*` — we use it deliberately
    // throughout the plugin error path. The structured logger
    // (`host.log.*`) is for plugins; kernel-side stays on console.
    "no-console": "off",

    // We use top-level `await` and ESM dynamic imports for sideloaded
    // plugins; Antfu's `node/prefer-global/process` and friends don't
    // apply (we're a browser/Wails-webview target, not Node).
    "node/prefer-global/process": "off",
    "node/no-process-env": "off",

    // We commonly write `if (!x) return;` as guard clauses (CLAUDE.md
    // explicitly endorses this). Antfu's curly preference is OK as-is
    // (allows omission for single-statement guards).

    // Unicorn rules that fight our style:
    "unicorn/prefer-node-protocol": "off", // Not Node.
    "unicorn/filename-case": "off",        // We use PascalCase + kebab-case mix.
    "unicorn/no-null": "off",              // null is meaningful in JSON/AG-UI payloads.
    "unicorn/prevent-abbreviations": "off", // ev / msg / ctx / refs are fine.

    // Test files set their own loose rules below; this just keeps the
    // top-level lint output clean.
    "antfu/no-import-dist": "off",
  },
}, {
  // Test files — looser. Vitest patterns sometimes need `any`,
  // non-null assertions, and unused helpers as decorations.
  files: ["**/*.test.{ts,tsx}", "**/test/**/*.{ts,tsx}"],
  rules: {
    "@typescript-eslint/no-explicit-any": "off",
    "@typescript-eslint/no-non-null-assertion": "off",
    "@typescript-eslint/no-unused-vars": "off",
    "no-unused-expressions": "off",
  },
});
