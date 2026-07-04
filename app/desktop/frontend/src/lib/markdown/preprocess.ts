// Math-delimiter + currency normalization applied to the accumulated markdown
// text BEFORE remark-math parses it. Language models routinely emit math in
// delimiters remark-math does not recognize — LaTeX `\(...\)` / `\[...\]`
// brackets and `[/math]` / `[/inline]` tags — and they write currency amounts
// (`$5`, `$19.99`) that single-`$` math would otherwise eat. These pure string
// transforms rewrite the alternative delimiters to the `$...$` / `$$...$$` form
// remark-math parses, and guard currency against the single-`$` parser.
//
// Streaming-safe: each runs on the full accumulated text and only rewrites
// COMPLETE delimiter pairs, so a half-arrived `\(` (no closing `\)` yet) is
// left untouched until its closer streams in. Pure + composable — apply the
// whole family via `normalizeMarkdownMath`.

// `\(...\)` inline. One OR two leading backslashes are accepted because models
// emit both depending on how the JSON transport escaped the response. No
// newline inside inline math (that would be a paragraph break, not one span).
const LATEX_INLINE_DELIMITER = /\\{1,2}\(([^\n]+?)\\{1,2}\)/g;
// `\[...\]` display. May span lines.
const LATEX_DISPLAY_DELIMITER = /\\{1,2}\[([\s\S]+?)\\{1,2}\]/g;

/**
 * Rewrites LaTeX bracket delimiters to dollar delimiters: `\(...\)` → `$...$`
 * (inline) and `\[...\]` → `$$...$$` (display). A single or double leading
 * backslash is accepted. remark-math only recognizes the dollar form, so
 * without this rewrite bracket math renders as literal text.
 */
export function rewriteLatexBracketDelimiters(text: string): string {
  return text
    .replace(LATEX_INLINE_DELIMITER, (_, body: string) => `$${body.trim()}$`)
    .replace(LATEX_DISPLAY_DELIMITER, (_, body: string) => `$$${body.trim()}$$`);
}

const MATH_TAG = /\[\/math\]([\s\S]*?)\[\/math\]/g;
const INLINE_TAG = /\[\/inline\]([\s\S]*?)\[\/inline\]/g;

/**
 * Rewrites the custom math tags some models emit to dollar delimiters:
 * `[/math]...[/math]` → `$$...$$` and `[/inline]...[/inline]` → `$...$`.
 */
export function rewriteCustomMathTags(text: string): string {
  return text
    .replace(MATH_TAG, (_, body: string) => `$$${body.trim()}$$`)
    .replace(INLINE_TAG, (_, body: string) => `$${body.trim()}$`);
}

// A `$` immediately before a digit is currency. Group 1 anchors on start-of-
// string or a char that is neither a backslash nor a `$` (so `$$` display math
// is left intact); group 2 captures an even-length run of backslashes so an
// already-escaped `\$` (odd run) is not double-escaped. A math expression
// almost always opens with a letter or a `\command`, so a digit after `$` is
// treated as currency.
const CURRENCY_DOLLAR = /(^|[^\\$])((?:\\\\)*)\$(?=\d)/g;

/**
 * Escapes a `$` immediately followed by a digit (`$5`, `$19.99`, `$1,299`) so
 * remark-math with single-`$` math enabled does not consume currency amounts in
 * prose as math delimiters. `$$` (display math) and an already-escaped `\$` are
 * preserved (the even backslash-run before the `$` is carried through).
 *
 * Trade-off: an inline expression that genuinely opens with a digit
 * (`$5x = 10$`) has its leading `$` escaped too. This is rare in practice; the
 * bracket rewrites above are unaffected because they run after escaping.
 */
export function escapeCurrencyDollars(text: string): string {
  return text.replace(CURRENCY_DOLLAR, "$1$2\\$");
}

/**
 * Rewrites the alternative math delimiters models emit (LaTeX brackets and
 * `[/math]` / `[/inline]` tags) to remark-math's `$...$` / `$$...$$` form.
 */
export function normalizeMathDelimiters(text: string): string {
  return rewriteLatexBracketDelimiters(rewriteCustomMathTags(text));
}

/**
 * The full preprocess: guard currency first, then normalize math delimiters.
 * Currency escaping runs BEFORE the bracket rewrites so a `\(5\)` (math that
 * opens with a digit) survives — at that point there is no `$` for the currency
 * rule to see, and the bracket rewrite then produces a correct `$5$`. Pure and
 * streaming-safe; apply to the whole accumulated body ahead of block-splitting.
 */
export function normalizeMarkdownMath(text: string): string {
  return normalizeMathDelimiters(escapeCurrencyDollars(text));
}
