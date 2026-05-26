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

// LRU-ish cache of highlighted HTML keyed by `(lang, theme, code)`.
// Re-rendering the same code block (scroll-away-and-back, theme toggle,
// MarkdownBlock memo invalidation in a long history) used to re-run the
// tokenizer every time. Both Cherry Studio (per-callerId LRU + Worker)
// and Portai (djb2 hash subscribers) cache this — the codeToHtml call
// is the heaviest single op in the streaming render path.
//
// Bounded so a long session can't grow the map unboundedly; FIFO
// eviction keeps the implementation tiny and the hot entries (top of
// the chat scroll, most recent code blocks) stay warm because they're
// re-inserted on access.
const CACHE_MAX = 128;
const cache = new Map<string, string>();

function cacheKey(lang: string, theme: string, code: string): string {
  return `${lang}\u0001${theme}\u0001${code}`;
}

export function getCachedHighlight(lang: string, theme: string, code: string): string | undefined {
  const key = cacheKey(lang, theme, code);
  const hit = cache.get(key);
  if (hit !== undefined) {
    // LRU touch — move to end so eviction targets cold entries first.
    cache.delete(key);
    cache.set(key, hit);
  }
  return hit;
}

export function setCachedHighlight(
  lang: string,
  theme: string,
  code: string,
  html: string,
): void {
  const key = cacheKey(lang, theme, code);
  cache.delete(key);
  cache.set(key, html);
  if (cache.size > CACHE_MAX) {
    // Drop the oldest insertion (Map iteration order = insertion order).
    const oldest = cache.keys().next().value;
    if (oldest !== undefined) cache.delete(oldest);
  }
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
