// Word-granularity text segmentation: Latin runs as words, CJK as
// individual codepoints, trailing punctuation glued to the preceding
// token, whitespace separate. Shared by useStreamReveal + rehypeFadeIn.
//
// Backed by Intl.Segmenter — standard since ES2022 (Chrome 87+, Safari
// 14.1+, FF 125+). Wails 2 ships modern WebView2 / WebKit so it's
// always available.

const segmenter = new Intl.Segmenter(undefined, { granularity: "word" });

const TRAILING_PUNCT_RE = /^[，。！？,!?]/;

export function segmentWords(text: string): string[] {
  const out: string[] = [];
  for (const { segment } of segmenter.segment(text)) {
    if (TRAILING_PUNCT_RE.test(segment) && out.length > 0) {
      out[out.length - 1] += segment;
    } else if (segment.length > 0) {
      out.push(segment);
    }
  }
  return out;
}
