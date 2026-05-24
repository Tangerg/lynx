// ESLint v9 flat config.
//
// Keeps the rule set deliberately tight — Lyra is a small project and a
// hundred bespoke rules become noise. The defaults from the recommended
// presets do 90% of the work; the rest are project-specific overrides.

import js from "@eslint/js";
import tseslint from "typescript-eslint";
import reactHooks from "eslint-plugin-react-hooks";
import reactRefresh from "eslint-plugin-react-refresh";

export default tseslint.config(
  {
    ignores: [
      "dist/**",
      "node_modules/**",
      "wailsjs/**",
      "src/lib/shiki.ts",
    ],
  },

  // TypeScript + JavaScript baseline
  js.configs.recommended,
  ...tseslint.configs.recommended,

  {
    files: ["**/*.{ts,tsx}"],
    plugins: {
      "react-hooks": reactHooks,
      "react-refresh": reactRefresh,
    },
    rules: {
      // React hooks rules — Lyra leans hard on hooks, this catches stale
      // closures and rules-of-hooks violations.
      ...reactHooks.configs.recommended.rules,
      // react-refresh/only-export-components: off — files in this repo
      // routinely export a component PLUS its props type or helper.
      // The dev-server Fast Refresh warning is noise.
      "react-refresh/only-export-components": "off",

      // Warnings — surface as work to do, not hard breaks.
      "@typescript-eslint/no-unused-vars": [
        "warn",
        { argsIgnorePattern: "^_", varsIgnorePattern: "^_" },
      ],
      "@typescript-eslint/no-explicit-any": "warn",

      // Off — too noisy for an evolving codebase.
      "@typescript-eslint/no-empty-object-type": "off",
      "@typescript-eslint/ban-ts-comment": "off",
      "@typescript-eslint/no-this-alias": "off",
      "no-empty-pattern": "off",
      "no-control-regex": "off",
      // The React 19 hooks lint preset adds these brand-new "refs /
      // components during render" rules. They false-positive a lot on
      // legitimate patterns (useMemo'd factories, ref-init-on-first-
      // render, stable hook factories) and we already cover the real
      // hazards via the long-standing rules-of-hooks + exhaustive-deps
      // checks.
      "react-hooks/refs": "off",
      "react-hooks/static-components": "off",
      "react-hooks/purity": "off",
    },
  },

  // Test files — allow `any`, allow non-null assertions, don't lint
  // unused helpers (vitest sometimes wants them as decorations).
  {
    files: ["**/*.test.{ts,tsx}", "**/test/**/*.{ts,tsx}"],
    rules: {
      "@typescript-eslint/no-explicit-any": "off",
      "@typescript-eslint/no-non-null-assertion": "off",
      "@typescript-eslint/no-unused-vars": "off",
    },
  },
);
