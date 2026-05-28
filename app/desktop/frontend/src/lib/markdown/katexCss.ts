// Lazy KaTeX stylesheet loader.
//
// `katex/dist/katex.min.css` is ~30KB of @font-face + .katex* rules.
// Eager-importing it (the previous setup) shipped that CSS to every
// user even when their session has no math at all — which is most
// sessions. Lazy-loading it through a dynamic import puts the CSS
// into its own chunk that the browser only fetches on the first
// math block we render.
//
// Cherry Studio does the same (lazy-loads KaTeX + MathJax engines on
// `$...$` detection); we just need the stylesheet because rehype-katex
// itself is already bundled.

let loaded = false;

/**
 * Ensure KaTeX styles are loaded. Idempotent + cheap to call repeatedly.
 * False positives (e.g. a literal `$100`) just trigger the load earlier;
 * the CSS injection is one-shot and the browser caches it.
 */
export function ensureKatexCss(): void {
  if (loaded) return;
  loaded = true;
  // Side-effect import — Vite emits this into its own asset chunk and
  // injects the resulting <link> when first evaluated.
  void import("./katexCssLoader");
}
