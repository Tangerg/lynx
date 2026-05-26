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
  /** Skip smoothing + fade-in. User-typed messages set this — the
   *  author already saw what they typed; animating it back is noise. */
  instant?: boolean;
}

// Module-level plugin lists keep react-markdown from treating each
// render as a new plugin set. Order matters in the rehype chain — see
// the MarkdownBlock comment for the pipeline.
const remarkPlugins = [remarkGfm, remarkBreaks, remarkCjkFriendly, remarkMath, remarkAlert];

// Tags that can execute / break sandbox even if the model emitted
// them as raw HTML — blocklist takes precedence over rehype-raw.
const DENIED_HTML_TAGS = new Set(["script", "iframe", "object", "embed", "form"]);
const allowElement = (el: { tagName: string }) => !DENIED_HTML_TAGS.has(el.tagName);

// MarkdownMessage — block-level memoised markdown renderer.
//
// We use Vercel `streamdown`'s tested `parseMarkdownIntoBlocks` (handles
// unclosed code fences / math / HTML tag balancing during streaming)
// but keep our own react-markdown + plugins + components map underneath
// — Streamdown's <Streamdown> ships its own `<span data-streamdown=
// "strong">` design system that bypasses `.md` CSS. Each block is its
// own memoised <MarkdownBlock>; only the tail block re-parses on each
// smooth-text tick.
export function MarkdownMessage({ text, streaming, instant }: Props) {
  const smoothed = useSmoothText(text, !instant && !!streaming);
  const display = instant ? text : smoothed;

  // remend (auto-close unterminated bold / inline code / fenced blocks)
  // runs on the *full* display before splitting — block boundaries read
  // more reliably on well-formed markdown. Skipped for instant messages.
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
        // Index keys are correct here: markdown blocks are append-only
        // during streaming, so position is a stable identity. Keying by
        // content would change the key on every tail-block edit and
        // force React to unmount + remount the fiber each tick — losing
        // useState / useEffect. With index keys the fiber survives and
        // `memo` decides re-render: completed blocks bail, tail block
        // runs the pipeline without the mount cost.
        <MarkdownBlock
          key={i}
          text={block}
          streaming={!!streaming && i === lastIdx}
          instant={instant}
        />
      ))}
    </div>
  );
}

// Single markdown block — paragraph / fence / list / heading. Memoised
// on its content + flags. `streaming` is forwarded but doesn't gate
// any plugin yet; kept on the signature for future per-block UI
// (e.g. a tail caret) that needs the bit.
const MarkdownBlock = memo(function MarkdownBlock({ text, instant }: Props) {
  // Pull in the KaTeX stylesheet (~30KB) the first time a math-bearing
  // block mounts. Probe is just `$` — false positives (USD prices)
  // preload the CSS earlier than strictly needed, which is harmless;
  // remarkMath itself ignores ambiguous single-`$` cases at render.
  const hasMath = text.includes("$");
  useEffect(() => {
    if (hasMath) ensureKatexCss();
  }, [hasMath]);

  // Pipeline: rehypeRaw (parse inline HTML) → rehypeCitations (swap
  // `[n]` markers for <sup> badges) → rehypeFadeIn (per-word streaming
  // animation, non-instant only — CSS runs once per span mount, so
  // settled blocks animate on first paint then stay inert) →
  // rehypeKatex. rehypeRaw must come first so later plugins see the
  // expanded tree.
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
