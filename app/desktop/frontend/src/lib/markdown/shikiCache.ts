// LRU-ish cache for Shiki-highlighted HTML.
//
// Streamdown's block-level memo means completed code blocks rarely
// re-render — but they DO re-mount on scroll-away/back, theme toggle,
// and when the parent MarkdownBlock's memo key invalidates in long
// histories. Each re-mount used to re-run the tokenizer (~3-10ms).
//
// Bounded so a long session can't grow the map unboundedly; FIFO
// eviction (Map insertion order = oldest first) keeps the impl tiny,
// and `get` re-inserts on hit so the hot tail of the chat stays warm.
//
// Both Cherry Studio (per-callerId LRU + Worker) and Portai (djb2 hash
// subscribers) cache this; this is the equivalent for our shiki path.

const CACHE_MAX = 128;
const cache = new Map<string, string>();

function cacheKey(lang: string, theme: string, code: string): string {
  return `${lang}\u0001${theme}\u0001${code}`;
}

export function getCachedHighlight(lang: string, theme: string, code: string): string | undefined {
  const key = cacheKey(lang, theme, code);
  const hit = cache.get(key);
  if (hit !== undefined) {
    // LRU touch — re-insert moves to end so FIFO eviction targets cold
    // entries first.
    cache.delete(key);
    cache.set(key, hit);
  }
  return hit;
}

export function setCachedHighlight(lang: string, theme: string, code: string, html: string): void {
  const key = cacheKey(lang, theme, code);
  cache.delete(key);
  cache.set(key, html);
  if (cache.size > CACHE_MAX) {
    // Map iteration order = insertion order; first key is oldest.
    const oldest = cache.keys().next().value;
    if (oldest !== undefined) cache.delete(oldest);
  }
}
