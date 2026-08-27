import { memo, useDeferredValue, useEffect, useMemo, useRef } from "react";

import ReactMarkdown, { defaultUrlTransform } from "react-markdown";
import rehypeKatex from "rehype-katex";
import rehypeRaw from "rehype-raw";
import remarkBreaks from "remark-breaks";
import remarkCjkFriendly from "remark-cjk-friendly";
import remarkGfm from "remark-gfm";
import remarkAlert from "remark-github-blockquote-alert";
import remarkMath from "remark-math";
import remend from "remend";
import { parseMarkdownIntoBlocks } from "streamdown";
import { createMarkdownComponents } from "./markdownComponents";
import { isInlineMarkdownImage } from "./MarkdownImage";
import { handleMarkdownCopy } from "./markdownSelectionCopy";
import { ensureKatexCss } from "./katexCss";
import { rehypeFadeIn } from "./rehypeFadeIn";
import { rehypeFileRefs } from "./rehypeFileRefs";
import { rehypeStreamCaret } from "./rehypeStreamCaret";
import { normalizeMarkdownMath } from "./preprocess";
import { remarkLiteralUnknownHtml } from "./remarkLiteralUnknownHtml";
import { useCommitThrottle, useStreamReveal } from "./streamReveal";
import { useVisibleTextMaterial } from "../messageVisibleMaterial";
import "remark-github-blockquote-alert/alert.css";

// Ceiling on how often the revealed text feeds the markdown re-parse while
// streaming. ~30fps: imperceptible for a text reveal, but caps a run of tiny
// tokens at one parse per window instead of one per animation frame.
const PARSE_COMMIT_MS = 33;

export type MarkdownReveal = "instant" | "smooth" | "typewriter";

type Props = {
  text: string;
} & (
  | {
      /** User-authored text: render immediately without replaying it to its author. */
      reveal: "instant";
      streaming?: false;
    }
  | {
      /** Assistant text: reveal whole words with fades, or characters with a caret. */
      reveal: Exclude<MarkdownReveal, "instant">;
      streaming?: boolean;
    }
);

interface MarkdownBlockProps {
  text: string;
  streaming: boolean;
  reveal: MarkdownReveal;
}

// Module-level plugin lists keep react-markdown from treating each
// render as a new plugin set. Order matters in the rehype chain — see
// the MarkdownBlock comment for the pipeline.
const remarkPlugins = [
  remarkGfm,
  remarkBreaks,
  remarkCjkFriendly,
  remarkMath,
  remarkAlert,
  remarkLiteralUnknownHtml,
];

// Tags that can execute / break sandbox even if the model emitted
// them as raw HTML — blocklist takes precedence over rehype-raw.
const DENIED_HTML_TAGS = new Set(["script", "iframe", "object", "embed", "form"]);
const allowElement = (el: { tagName: string }) => !DENIED_HTML_TAGS.has(el.tagName);

// react-markdown intentionally drops data URLs by default. Desktop blocks every
// remote Markdown image in MarkdownImage, but explicitly inlined image data is
// already self-contained and must survive the parser to reach that renderer.
const markdownUrlTransform: NonNullable<
  React.ComponentProps<typeof ReactMarkdown>["urlTransform"]
> = (value, _key, node) =>
  node.tagName === "img" && isInlineMarkdownImage(value) ? value : defaultUrlTransform(value);

// MarkdownMessage — block-level memoised markdown renderer.
//
// We use Vercel `streamdown`'s tested `parseMarkdownIntoBlocks` (handles
// unclosed code fences / math / HTML tag balancing during streaming)
// but keep our own react-markdown + plugins + components map underneath
// — Streamdown's <Streamdown> ships its own `<span data-streamdown=
// "strong">` design system that bypasses `.md` CSS. Each block is its
// own memoised <MarkdownBlock>; only the tail block re-parses on each
// stream-reveal tick.
export function MarkdownMessage(props: Props) {
  const { text, reveal } = props;
  const streaming = reveal === "instant" ? false : !!props.streaming;
  const instant = reveal === "instant";
  const rootRef = useRef<HTMLDivElement>(null);
  const revealed = useStreamReveal(text, streaming, reveal === "typewriter");
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
  useVisibleTextMaterial(source === text);

  // Normalize model-emitted math delimiters + guard currency BEFORE remark-math
  // parses. Must run on the whole body ahead of block-splitting so a display
  // math span (`$$...$$`) isn't torn across two blocks.
  const normalized = useMemo(() => normalizeMarkdownMath(source), [source]);

  // remend (auto-close unterminated bold / inline code / fenced blocks)
  // runs on the *full* text before splitting — block boundaries read
  // more reliably on well-formed markdown. Skipped for instant messages.
  const repaired = useMemo(() => {
    if (instant) return normalized;
    return remend(normalized);
  }, [instant, normalized]);

  const blocks = useMemo(() => parseMarkdownIntoBlocks(repaired), [repaired]);
  const lastIdx = blocks.length - 1;

  useEffect(() => {
    const root = rootRef.current;
    if (!root) return;
    const ownerDocument = root.ownerDocument;
    const onCopy = (event: ClipboardEvent) => {
      handleMarkdownCopy(root, event);
    };
    ownerDocument.addEventListener("copy", onCopy, true);
    return () => ownerDocument.removeEventListener("copy", onCopy, true);
  }, []);

  return (
    <div ref={rootRef} className="md" dir="auto">
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
          streaming={streaming && i === lastIdx}
          reveal={reveal}
        />
      ))}
    </div>
  );
}

// Single markdown block — paragraph / fence / list / heading. Memoised
// on its content + flags. In smooth mode the per-word fade-in conveys
// "currently generating"; in typewriter mode `streaming` (true only for
// the tail block) gates the blinking accent caret instead.
const MarkdownBlock = memo(function MarkdownBlock({ text, streaming, reveal }: MarkdownBlockProps) {
  // Pull in the KaTeX stylesheet (~30KB) the first time a math-bearing
  // block mounts. Probe is just `$` — false positives (USD prices)
  // preload the CSS earlier than strictly needed, which is harmless;
  // remarkMath itself ignores ambiguous single-`$` cases at render.
  const hasMath = text.includes("$");
  useEffect(() => {
    if (hasMath) ensureKatexCss();
  }, [hasMath]);

  // Pipeline: rehypeRaw (parse inline HTML) → rehypeFileRefs (linkify file:line) →
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
    if (reveal === "instant") {
      return [rehypeRaw, rehypeFileRefs, rehypeKatex];
    }
    if (reveal === "typewriter") {
      return streaming
        ? [rehypeRaw, rehypeKatex, rehypeStreamCaret]
        : [rehypeRaw, rehypeFileRefs, rehypeKatex];
    }
    return streaming
      ? [rehypeRaw, rehypeFadeIn, rehypeKatex]
      : [rehypeRaw, rehypeFileRefs, rehypeFadeIn, rehypeKatex];
  }, [reveal, streaming]);
  const components = useMemo(() => createMarkdownComponents(text), [text]);

  return (
    <ReactMarkdown
      remarkPlugins={remarkPlugins}
      rehypePlugins={rehypePlugins}
      components={components}
      allowElement={allowElement}
      urlTransform={markdownUrlTransform}
    >
      {text}
    </ReactMarkdown>
  );
});
