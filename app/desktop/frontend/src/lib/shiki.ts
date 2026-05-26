// Lazy singleton Shiki highlighter.
//
// Shiki's `createHighlighter` is async (loads grammars / themes from
// bundled JSON). We create it once on first request and share the same
// instance across the whole app. Themes follow the app's light/dark
// scheme; languages are a curated list covering what an LLM is likely
// to emit in chat. Adding a language is cheap — extend `LANGS` below.
//
// The `shiki` module itself is also dynamic-imported so the core
// (~400KB) and its grammar JSONs don't ship in the main chunk;
// they're fetched the first time a code block actually renders.

import type { Highlighter } from "shiki";

const THEMES = ["github-dark", "github-light"] as const;

const LANGS = [
  "typescript",
  "javascript",
  "tsx",
  "jsx",
  "python",
  "go",
  "rust",
  "java",
  "c",
  "cpp",
  "csharp",
  "ruby",
  "php",
  "swift",
  "kotlin",
  "bash",
  "shell",
  "json",
  "yaml",
  "toml",
  "html",
  "css",
  "scss",
  "markdown",
  "sql",
  "diff",
  "dockerfile",
  "graphql",
  "xml",
] as const;

let promise: Promise<Highlighter> | null = null;

export function getHighlighter(): Promise<Highlighter> {
  if (promise === null) {
    promise = import("shiki").then(({ createHighlighter }) =>
      createHighlighter({
        themes: [...THEMES],
        langs: [...LANGS],
      }),
    );
  }
  return promise;
}

// Pick the closest loaded language for a tag — Shiki throws on unknown
// langs, so we degrade to plain "text" if the model emits something we
// don't bundle (e.g., `kdl`, `nix`).
export function resolveLang(highlighter: Highlighter, lang: string): string {
  const loaded = new Set(highlighter.getLoadedLanguages());
  if (loaded.has(lang)) return lang;
  // Common aliases the model might use.
  const aliases: Record<string, string> = {
    ts: "typescript",
    js: "javascript",
    py: "python",
    rb: "ruby",
    rs: "rust",
    sh: "bash",
    zsh: "bash",
    yml: "yaml",
    dockerfile: "dockerfile",
    docker: "dockerfile",
    "c++": "cpp",
    "c#": "csharp",
    cs: "csharp",
  };
  const aliased = aliases[lang.toLowerCase()];
  if (aliased && loaded.has(aliased)) return aliased;
  return "text";
}
