import { useMemo } from "react";
import ReactMarkdown from "react-markdown";
import remarkBreaks from "remark-breaks";
import remarkCjkFriendly from "remark-cjk-friendly";
import remarkGfm from "remark-gfm";
import remend from "remend";
import { markdownComponents } from "@/components/chat/markdownComponents";
import { rehypeFadeIn } from "@/lib/rehypeFadeIn";
import { useSmoothText } from "@/lib/smoothText";

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
// as a new plugin set. remarkBreaks turns a single \n into <br> (LLMs
// expect that); remarkCjkFriendly fixes bold/italic boundaries that
// vanilla CommonMark breaks for CJK text.
const remarkPlugins = [remarkGfm, remarkBreaks, remarkCjkFriendly];

// MarkdownMessage — full Markdown with optional smooth-streamed per-
// word fade-in. Pipeline: raw → useSmoothText → remend (auto-close
// open `, **, ``` mid-stream) → react-markdown → rehypeFadeIn → JSX.
// `instant=true` skips smoothing + fade-in for user-typed messages.
export function MarkdownMessage({ text, streaming, instant }: Props) {
  const smoothed = useSmoothText(text, !instant && !!streaming);
  const display = instant ? text : smoothed;
  // remend handles open code/math fences + word-internal context
  // (`foo_bar` isn't an unterminated italic) so partial markdown
  // doesn't render as broken syntax mid-stream.
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
