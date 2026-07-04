import { memo, useDeferredValue, useEffect, useMemo } from "react";

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
import { markdownComponents } from "./markdownComponents";
import { ensureKatexCss } from "@/lib/markdown/katexCss";
import { measureMarkdownRepair } from "@/lib/metrics";
import { rehypeCitations } from "@/lib/markdown/rehypeCitations";
import { rehypeFadeIn } from "@/lib/markdown/rehypeFadeIn";
import { rehypeFileRefs } from "./rehypeFileRefs";
import { rehypeStreamCaret } from "@/lib/markdown/rehypeStreamCaret";
import { normalizeMarkdownMath } from "@/lib/markdown/preprocess";
import { useCommitThrottle, useStreamReveal } from "./streamReveal";
import "remark-github-blockquote-alert/alert.css";

// Ceiling on how often the revealed text feeds the markdown re-parse while
// streaming. ~30fps: imperceptible for a text reveal, but caps a run of tiny
// tokens at one parse per window instead of one per animation frame.
const PARSE_COMMIT_MS = 33;

interface Props {
  text: string;
  streaming?: boolean;
  /** Skip smoothing + fade-in. User-typed messages set this — the
   *  author already saw what they typed; animating it back is noise. */
  instant?: boolean;
  /** Reveal char-by-char (typewriter) instead of word + per-word fade.
   *  Drops the fade-in so each character appears crisp. */
  typewriter?: boolean;
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
// stream-reveal tick.
export function MarkdownMessage({ text, streaming, instant, typewriter }: Props) {
  const revealed = useStreamReveal(text, !instant && !!streaming, typewriter);
  const display = instant ? text : revealed;

  // Cap the re-parse frequency during streaming (the reveal ticks ~60×/s but
  // the eye can't resolve that on a text crawl). Passthrough for instant text.
  const committed = useCommitThrottle(display, streaming ? PARSE_COMMIT_MS : 0);

  // useDeferredValue lets React re-parse a long body at low priority: scrolling
  // and typing keep the previous parse on-screen instead of blocking a frame on
  // the new one. Instant (user-typed, settled) text skips the defer to stay
  // crisp on first paint — there's no stream to keep responsive.
  const deferred = useDeferredValue(committed);
  const source = instant ? committed : deferred;

  // Normalize model-emitted math delimiters + guard currency BEFORE remark-math
  // parses. Must run on the whole body ahead of block-splitting so a display
  // math span (`$$...$$`) isn't torn across two blocks.
  const normalized = useMemo(() => normalizeMarkdownMath(source), [source]);

  // remend (auto-close unterminated bold / inline code / fenced blocks)
  // runs on the *full* text before splitting — block boundaries read
  // more reliably on well-formed markdown. Skipped for instant messages.
  const repaired = useMemo(() => {
    if (instant) return normalized;
    const start = performance.now();
    const out = remend(normalized);
    measureMarkdownRepair(performance.now() - start, normalized.length, !!streaming);
    return out;
  }, [normalized, streaming, instant]);

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
          typewriter={typewriter}
        />
      ))}
    </div>
  );
}

// Single markdown block — paragraph / fence / list / heading. Memoised
// on its content + flags. In smooth mode the per-word fade-in conveys
// "currently generating"; in typewriter mode `streaming` (true only for
// the tail block) gates the blinking accent caret instead.
const MarkdownBlock = memo(function MarkdownBlock({ text, instant, streaming, typewriter }: Props) {
  // Pull in the KaTeX stylesheet (~30KB) the first time a math-bearing
  // block mounts. Probe is just `$` — false positives (USD prices)
  // preload the CSS earlier than strictly needed, which is harmless;
  // remarkMath itself ignores ambiguous single-`$` cases at render.
  const hasMath = text.includes("$");
  useEffect(() => {
    if (hasMath) ensureKatexCss();
  }, [hasMath]);

  // Pipeline: rehypeRaw (parse inline HTML) → rehypeCitations (swap
  // `[n]` markers for <sup> badges) → rehypeFileRefs (linkify file:line) →
  // rehypeFadeIn (per-word streaming animation, non-instant only — CSS runs
  // once per span mount, so settled blocks animate on first paint then stay
  // inert) → rehypeKatex. rehypeRaw must come first so later plugins see the
  // expanded tree. Typewriter mode drops rehypeFadeIn — the char-by-char
  // reveal is the animation, a per-word fade on top would muddy it — and adds
  // a blinking accent caret on the streaming tail block instead.
  //
  // rehypeFileRefs runs only on a SETTLED block (never the streaming tail): a
  // half-arrived path would flash as a link, and it must precede rehypeFadeIn
  // so it sees whole text nodes, not per-word spans. Instant (user-typed)
  // blocks are settled by definition, so they always linkify.
  const rehypePlugins = useMemo(() => {
    if (instant) return [rehypeRaw, rehypeCitations, rehypeFileRefs, rehypeKatex];
    if (typewriter) {
      return streaming
        ? [rehypeRaw, rehypeCitations, rehypeKatex, rehypeStreamCaret]
        : [rehypeRaw, rehypeCitations, rehypeFileRefs, rehypeKatex];
    }
    return streaming
      ? [rehypeRaw, rehypeCitations, rehypeFadeIn, rehypeKatex]
      : [rehypeRaw, rehypeCitations, rehypeFileRefs, rehypeFadeIn, rehypeKatex];
  }, [instant, typewriter, streaming]);

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
