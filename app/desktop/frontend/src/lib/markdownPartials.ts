// Markdown stream-mode helpers.
//
// When markdown is streamed character-by-character from an LLM, the
// source spends most of its lifetime in a "partial" state: an opening
// ``` arrives long before the closing ``` does, a single `` ` `` opens
// the inline-code span ahead of its closer, etc. react-markdown is a
// strict parser — without a closer it gives up and renders the partial
// as literal backticks / asterisks until the closer streams in, then
// snaps the AST into a code block / inline code / bold span. That snap
// is visually jarring during streaming.
//
// `closeOpenMarkers` is the cheap fix: count the openers, append a
// matching closer when the count is odd, and let react-markdown render
// the synthetic-but-valid markdown. As the real closer arrives the
// synthetic one becomes a no-op (one more `` ` `` cancels another) and
// the AST stays stable across the transition.

// Cap how many synthetic closers we'll append in pathological inputs.
// Guards against truly malformed streams (e.g. 99 unmatched markers)
// blowing past O(n) without ever stabilizing.
const MAX_PATCH_PASSES = 4;

export function closeOpenMarkers(source: string): string {
  let out = source;
  for (let pass = 0; pass < MAX_PATCH_PASSES; pass++) {
    const next = closeOnce(out);
    if (next === out) break;
    out = next;
  }
  return out;
}

function closeOnce(s: string): string {
  let out = s;

  // 1) Fenced code blocks. Count ``` occurrences; odd → synthesise a
  //    closer on its own line. The opener-with-no-closer pattern is the
  //    most common partial mid-stream.
  const fenceCount = (out.match(/```/g) ?? []).length;
  if (fenceCount % 2 === 1) {
    out = out.endsWith("\n") ? out + "```" : out + "\n```";
  }

  // 2) Inline backticks. Strip fences first (already balanced above)
  //    so single ticks inside a code block aren't double-counted.
  const noFences = out.replace(/```[\s\S]*?```/g, "").replace(/```/g, "");
  const ticks = (noFences.match(/`/g) ?? []).length;
  if (ticks % 2 === 1) {
    out = out + "`";
  }

  // 3) Bold ** pairs. Strip inline-code regions first so backticked
  //    `**` doesn't contribute to the count.
  const noCode = out.replace(/`[^`]*`/g, "");
  const stars = (noCode.match(/\*\*/g) ?? []).length;
  if (stars % 2 === 1) {
    out = out + "**";
  }

  return out;
}
