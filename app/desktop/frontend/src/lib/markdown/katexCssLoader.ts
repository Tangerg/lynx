// Side-effect-only module: importing this evaluates the KaTeX
// stylesheet import, which Vite turns into a `<link>` injection.
// Kept as a separate file so the dynamic-import target is small and
// Vite splits it into its own chunk (the rest of `katexCss.ts` stays
// in the main chunk for synchronous `ensureKatexCss()` access).
import "katex/dist/katex.min.css";
