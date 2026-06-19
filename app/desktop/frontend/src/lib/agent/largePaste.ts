// Heuristic for "this paste is big enough to collapse into an attachment chip
// instead of dumping it raw into the textarea" (T2.3). A small snippet stays
// inline so the user can see / edit it; a pasted file / stack trace / log that
// would balloon the composer becomes a removable chip and is re-inlined into
// the message only on send. Mirrors the codex / Claude Code large-paste UX.
//
// Either bound trips it: a tall paste (many lines, which scrolls the capped
// textarea) OR a wide one (a long single-line minified blob). Conservative —
// a miss just leaves the text inline, never blocks anything.

export const LARGE_PASTE_LINES = 12;
export const LARGE_PASTE_CHARS = 1600;

/** Number of lines in `text` (1 for a newline-free string). */
export function countLines(text: string): number {
  return text.split("\n").length;
}

/** Whether a pasted string should be collapsed into an attachment chip. */
export function isLargePaste(text: string): boolean {
  return text.length >= LARGE_PASTE_CHARS || countLines(text) >= LARGE_PASTE_LINES;
}
