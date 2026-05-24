import { useMemo } from "react";
import ReactMarkdown from "react-markdown";
import remarkGfm from "remark-gfm";
import remarkBreaks from "remark-breaks";
import remarkCjkFriendly from "remark-cjk-friendly";
import remend from "remend";
import { useSmoothText } from "@/lib/smoothText";
import { rehypeFadeIn } from "@/lib/rehypeFadeIn";
import { markdownComponents } from "@/components/chat/markdownComponents";

type Props = {
  text: string;
  streaming?: boolean;
  /**
   * Render synchronously without smoothing or fade-in. Used for
   * user-typed messages — the author already saw what they typed, so
   * animating it back feels patronizing.
   */
  instant?: boolean;
};

// remark plugins are stable across renders; declaring outside the
// component keeps react-markdown from treating each render as a new
// plugin set.
//
//   remarkGfm           → tables, task lists, strikethrough, autolinks
//   remarkBreaks        → single \n becomes <br> (matches LLM output
//                         conventions; without it most paragraphs feel
//                         glued together)
//   remarkCjkFriendly   → CJK-aware bold/italic/strikethrough boundaries
//                         (vanilla CommonMark requires whitespace around
//                         **/*/~~, which breaks for inline emphasis
//                         inside Chinese / Japanese / Korean text)
const remarkPlugins = [remarkGfm, remarkBreaks, remarkCjkFriendly];

// MarkdownMessage — renders one message's body as full Markdown, with
// optional smooth-streamed per-word fade-in.
//
// Pipeline:
//   raw text
//     → useSmoothText (paces the reveal at ~40-160 chars/sec)
//     → remend (auto-closes ```, `, **, links, etc. mid-stream)
//     → react-markdown + remark-gfm (tables, task lists, strikethrough)
//     → rehype: rehypeFadeIn wraps non-code text nodes in
//       <span class="fade-in">word</span>
//     → component map (markdownComponents): code blocks → Shiki,
//       mermaid blocks → SVG
//     → JSX
//
// React reconciles by element position; since the smoothed prefix is
// strictly append-only, existing word-spans retain stable keys across
// renders and don't replay their fade-in. Only the newly-appended tail
// gets a fresh mount → fresh animation.
//
// When `instant` is true, smoothing is bypassed and rehypeFadeIn is
// dropped from the pipeline — the text renders as plain Markdown with
// zero animation (used for user-typed messages).
export function MarkdownMessage({ text, streaming, instant }: Props) {
  // Hook must run unconditionally; when `instant`, pass enabled=false so
  // it jumps to full length and idles.
  const smoothed = useSmoothText(text, !instant && !!streaming);
  const display = instant ? text : smoothed;
  // `remend` is the same job our old closeOpenMarkers did, but
  // production-grade: it knows about code/math blocks, links, images,
  // strikethrough, and word-internal context (so `foo_bar` isn't
  // treated as an unterminated italic, etc.). Same author as
  // use-stick-to-bottom — drop-in upgrade.
  const safe = useMemo(() => remend(display), [display]);

  const rehypePlugins = useMemo(() => (instant ? [] : [rehypeFadeIn]), [instant]);

  return (
    <div className="md">
      <ReactMarkdown
        remarkPlugins={remarkPlugins}
        rehypePlugins={rehypePlugins}
        components={markdownComponents}
      >
        {safe}
      </ReactMarkdown>
    </div>
  );
}
