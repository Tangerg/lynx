// Word-granularity text segmentation.
//
// Latin runs and CJK individual codepoints, trailing punctuation glued
// to the preceding token. Whitespace stays separate so the render layer
// can pass it through as inert text.
//
// Shared by:
//   - useSmoothText  (paces reveal by whole-word units)
//   - rehypeFadeIn   (wraps non-code text nodes in per-word fade spans)
//
// Lives in its own file so neither caller pulls the other in just to
// share these helpers. Before this split, rehypeFadeIn had to import
// from `smoothText.ts` — a React-hook module — purely for `segmentWords`,
// which left the rehype plugin coupled to a hook it never used.

export function segmentWords(text: string): string[] {
  if (typeof Intl !== "undefined" && "Segmenter" in Intl) {
    try {
      const seg = new Intl.Segmenter(undefined, { granularity: "word" });
      const out: string[] = [];
      for (const { segment } of seg.segment(text)) {
        if (/^[，。！？,!?]/.test(segment) && out.length > 0) {
          out[out.length - 1] += segment;
        } else {
          out.push(segment);
        }
      }
      return out.filter((s) => s.length > 0);
    } catch {
      /* fall through to regex */
    }
  }
  // Fallback when Intl.Segmenter is unavailable.
  const tokens: string[] = [];
  const re =
    /(\[[^\]]*\])|([a-zA-Z0-9]+[，。！？,!?]*)|(\p{Unified_Ideograph}[，。！？,!?]*)|(\s+)|(.)/gsu;
  let m: RegExpExecArray | null;
  while ((m = re.exec(text)) !== null) tokens.push(m[0]);
  return tokens;
}
