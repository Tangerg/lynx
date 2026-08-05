/**
 * Program output carries SGR escapes — `go test`, npm, eslint and every linter a
 * coding agent runs colour their output — and until now they were drawn as text: a
 * failing test read `[0;31mFAIL[0m` instead of a red FAIL.
 *
 * Hand-written rather than `anser` / `ansi-to-html`, and the reason is the palette.
 * Those emit inline styles or an HTML string carrying literal colours, which is the
 * one thing this app cannot accept: a literal cannot follow the scheme, the contrast
 * preference or a contributed theme, and ANSI red on a dark well at whatever hex a
 * library picked is exactly the class of failure the tone tokens exist to prevent.
 * What comes out of here is spans plus a TONE, and the renderer dresses them.
 */

/** Which of the app's tones an SGR colour maps to. */
export type AnsiTone = "negative" | "success" | "warning" | "info" | "accent" | "muted";

export interface AnsiSpan {
  text: string;
  tone?: AnsiTone;
  bold?: boolean;
  dim?: boolean;
  underline?: boolean;
}

// The eight SGR colours onto the tones this app has. Cyan and magenta have no tone of
// their own here — mapping them to `info` and `accent` keeps every colour on the
// ramp, which is what makes them legible in both schemes. Bright variants (90–97)
// share their base tone: brightness in a terminal is a second axis this palette
// expresses with weight instead.
const TONE_BY_SGR: Record<number, AnsiTone> = {
  30: "muted",
  31: "negative",
  32: "success",
  33: "warning",
  34: "info",
  35: "accent",
  36: "info",
  37: "muted",
};

// oxlint-disable-next-line no-control-regex -- the escape byte is the thing being matched
const CSI = /\u001b\[([0-9;?]*)([A-Za-z])/g;

interface Style {
  tone?: AnsiTone;
  bold?: boolean;
  dim?: boolean;
  underline?: boolean;
}

function applySgr(style: Style, params: string): Style {
  // An empty parameter list means SGR 0.
  const codes = params === "" ? [0] : params.split(";").map((p) => Number.parseInt(p, 10) || 0);
  let next = { ...style };
  for (let i = 0; i < codes.length; i += 1) {
    const code = codes[i]!;
    if (code === 0) next = {};
    else if (code === 1) next.bold = true;
    else if (code === 2) next.dim = true;
    else if (code === 4) next.underline = true;
    else if (code === 22) {
      next.bold = undefined;
      next.dim = undefined;
    } else if (code === 24) next.underline = undefined;
    else if (code === 39) next.tone = undefined;
    else if (code in TONE_BY_SGR) next.tone = TONE_BY_SGR[code];
    else if (code >= 90 && code <= 97) next.tone = TONE_BY_SGR[code - 60];
    // 256-colour and truecolour selectors carry their own arguments; skip them
    // rather than read the arguments as further codes.
    else if (code === 38 || code === 48) i += codes[i + 1] === 5 ? 2 : 4;
  }
  return next;
}

/**
 * Split text into styled spans, dropping every escape sequence that is not a colour.
 *
 * Cursor moves and erases are dropped rather than honoured: this is a transcript, not
 * a terminal — there is no cursor to move, and a progress bar that redraws itself with
 * `\r` and `ESC[K` would otherwise leave its intermediate frames stacked in the log.
 */
export function parseAnsi(input: string): AnsiSpan[] {
  const spans: AnsiSpan[] = [];
  let style: Style = {};
  let cursor = 0;

  const push = (text: string) => {
    if (text === "") return;
    const last = spans[spans.length - 1];
    // Merge with the previous span when the style is unchanged, so a line broken up
    // by resets does not become a span per word.
    if (
      last &&
      last.tone === style.tone &&
      last.bold === style.bold &&
      last.dim === style.dim &&
      last.underline === style.underline
    ) {
      last.text += text;
      return;
    }
    spans.push({ text, ...style });
  };

  CSI.lastIndex = 0;
  for (let match = CSI.exec(input); match !== null; match = CSI.exec(input)) {
    push(input.slice(cursor, match.index));
    if (match[2] === "m") style = applySgr(style, match[1] ?? "");
    cursor = match.index + match[0].length;
  }
  push(input.slice(cursor));
  return spans;
}

/** Whether the text carries any escape sequence at all — the cheap check a caller
 *  uses to skip the parse for the overwhelmingly common plain case. */
export function hasAnsi(input: string): boolean {
  return input.includes("\u001b");
}
