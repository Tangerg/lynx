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
//
// Tokenizer output caching lives next door in `shikiCache.ts`.

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

// Map a file path to a Shiki language tag by extension (or a bare basename like
// "Dockerfile" / "Makefile"). Returns "text" for anything unrecognized; pass the
// result through [resolveLang] before use so an un-bundled tag still degrades
// cleanly. Used by the diff view to highlight each file in its OWN language
// rather than assuming one.
export function langFromPath(path: string): string {
  const base = path.slice(path.lastIndexOf("/") + 1);
  const byName: Record<string, string> = {
    Dockerfile: "dockerfile",
    Makefile: "bash", // close enough for tab-indented recipes
  };
  if (byName[base]) return byName[base]!;
  const ext = base.slice(base.lastIndexOf(".") + 1).toLowerCase();
  const byExt: Record<string, string> = {
    ts: "typescript",
    tsx: "tsx",
    mts: "typescript",
    cts: "typescript",
    js: "javascript",
    mjs: "javascript",
    cjs: "javascript",
    jsx: "jsx",
    py: "python",
    go: "go",
    rs: "rust",
    java: "java",
    c: "c",
    h: "c",
    cc: "cpp",
    cpp: "cpp",
    cxx: "cpp",
    hpp: "cpp",
    cs: "csharp",
    rb: "ruby",
    php: "php",
    swift: "swift",
    kt: "kotlin",
    kts: "kotlin",
    sh: "bash",
    bash: "bash",
    zsh: "bash",
    json: "json",
    jsonc: "json",
    yaml: "yaml",
    yml: "yaml",
    toml: "toml",
    html: "html",
    htm: "html",
    css: "css",
    scss: "scss",
    md: "markdown",
    markdown: "markdown",
    sql: "sql",
    graphql: "graphql",
    gql: "graphql",
    xml: "xml",
  };
  return byExt[ext] ?? "text";
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
