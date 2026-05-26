import { memo, useEffect, useMemo } from "react";

import ReactMarkdown from "react-markdown";
import rehypeKatex from "rehype-katex";
import rehypeRaw from "rehype-raw";
import remarkBreaks from "remark-breaks";
import remarkCjkFriendly from "remark-cjk-friendly";
import remarkGfm from "remark-gfm";
import remarkAlert from "remark-github-blockquote-alert";
import remarkMath from "remark-math";
import remend from "remend";
import { parseMarkdownIntoBlocks } from "streamdown";
import { markdownComponents } from "@/components/chat/markdownComponents";
import { ensureKatexCss } from "@/lib/katexCss";
import { measureMarkdownRepair } from "@/lib/metrics";
import { rehypeCitations } from "@/lib/rehypeCitations";
import { rehypeFadeIn } from "@/lib/rehypeFadeIn";
import { useSmoothText } from "@/lib/smoothText";
import "remark-github-blockquote-alert/alert.css";

interface Props {
  text: string;
  streaming?: boolean;
  /**
   * Render synchronously without smoothing or fade-in. Used for
   * user-typed messages — the author already saw what they typed, so
   * animating it back feels patronizing.
   */
  instant?: boolean;
}

// Stable plugin list — keeps react-markdown from treating each render
// as a new plugin set.
//   remarkGfm           — tables / strikethrough / task lists
//   remarkBreaks        — single \n → <br> (LLMs expect that)
//   remarkCjkFriendly   — fix bold/italic boundaries on CJK text
//   remarkMath          — parse $…$ / $$…$$ blocks
//   remarkAlert         — GitHub `> [!NOTE]` / [!WARNING] / etc. callouts
const remarkPlugins = [remarkGfm, remarkBreaks, remarkCjkFriendly, remarkMath, remarkAlert];

// rehypeRaw lets the markdown pipe parse inline HTML — `<details>`,
// `<kbd>`, `<sub>`, `<sup>`, `<mark>`, `<table>` etc. that agents
// regularly emit. We pass a strict allow list (`allowElement`) below
// so scripts / iframes / objects / embeds can't sneak through.
const DENIED_HTML_TAGS = new Set(["script", "iframe", "object", "embed", "form"]);

const allowElement = (el: { tagName: string }) => !DENIED_HTML_TAGS.has(el.tagName);

// MarkdownMessage — full Markdown with optional smooth-streamed per-
// word fade-in.
//
// Renderer strategy: borrow Vercel `streamdown`'s tested block splitter
// (`parseMarkdownIntoBlocks` — handles unclosed code fences / math /
// HTML tag balancing), but keep our own react-markdown + plugins +
// components map underneath. Each block is its own memoised
// `<MarkdownBlock>`, so on every smooth-text tick only the tail block
// re-runs the remark→rehype→react pipeline; completed blocks stay
// inert.
//
// Why not the full <Streamdown> component: it ships its own
// `<span data-streamdown="strong">` design system that bypasses
// `.md *` CSS rules we already maintain. The splitter alone is the
// piece worth keeping.
export function MarkdownMessage({ text, streaming, instant }: Props) {
  const smoothed = useSmoothText(text, !instant && !!streaming);
  const display = instant ? text : smoothed;

  // remend (auto-close unterminated bold / inline code / fenced blocks)
  // runs on the *full* display text before splitting — block boundaries
  // are easier to detect on well-formed markdown. Skipped for instant
  // messages (user input is always complete).
  const repaired = useMemo(() => {
    if (instant) return display;
    const start = performance.now();
    const out = remend(display);
    measureMarkdownRepair(performance.now() - start, display.length, !!streaming);
    return out;
  }, [display, streaming, instant]);

  const blocks = useMemo(() => parseMarkdownIntoBlocks(repaired), [repaired]);
  const lastIdx = blocks.length - 1;

  return (
    <div className="md">
      {blocks.map((block, i) => (
        <MarkdownBlock
          // Block content is the stable identity — re-keying by index
          // would remount the tail block every render and lose its
          // memo. By keying on content, completed blocks keep their
          // React fibers across re-renders.
          key={`${i}:${block}`}
          text={block}
          // Only the tail block is "streaming" — earlier blocks have
          // settled, no fade-in / no caret pulses needed.
          streaming={!!streaming && i === lastIdx}
          instant={instant}
        />
      ))}
    </div>
  );
}

// Cheap "this block contains math" probe. A literal `$` is enough —
// false positives (a paragraph mentioning USD prices) just preload
// the KaTeX stylesheet earlier than strictly necessary, which is
// harmless. remarkMath / rehype-katex won't actually render anything
// without proper `$…$` pairs.
function blockHasMath(text: string): boolean {
  return text.includes("$");
}

// Single block of markdown — paragraph / fenced code / list / heading.
// Memoised on its content + flags so a re-render of the parent that
// doesn't change this block's text skips the entire react-markdown
// pipeline for it. `streaming` is forwarded but doesn't currently
// gate any plugin — kept in the signature so future per-block UI
// (e.g. a tail caret) has a clean prop to read.
const MarkdownBlock = memo(function MarkdownBlock({ text, instant }: Props) {
  // Pull in the KaTeX stylesheet the first time a math-bearing block
  // mounts. Skips the ~30KB CSS for math-free sessions entirely.
  const hasMath = blockHasMath(text);
  useEffect(() => {
    if (hasMath) ensureKatexCss();
  }, [hasMath]);
  // Pipeline: rehypeRaw (parse inline HTML) → rehypeCitations (swap
  // `[n]` markers for <sup> badges) → rehypeFadeIn (per-word streaming
  // animation, non-instant only — the CSS animation runs once per
  // span mount, so settled blocks animate on first paint then stay
  // inert) → rehypeKatex (math). rehypeRaw must come first so later
  // rehype plugins see the expanded tree.
  const rehypePlugins = useMemo(
    () =>
      instant
        ? [rehypeRaw, rehypeCitations, rehypeKatex]
        : [rehypeRaw, rehypeCitations, rehypeFadeIn, rehypeKatex],
    [instant],
  );

  return (
    <ReactMarkdown
      remarkPlugins={remarkPlugins}
      rehypePlugins={rehypePlugins}
      components={markdownComponents}
      allowElement={allowElement}
    >
      {text}
    </ReactMarkdown>
  );
});
