// Word-granularity text segmentation: Latin runs as words, CJK as
// individual codepoints, trailing punctuation glued to the preceding
// token, whitespace separate. Shared by useSmoothText + rehypeFadeIn.

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
