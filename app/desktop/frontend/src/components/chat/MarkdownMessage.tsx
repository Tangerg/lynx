import { useMemo } from "react";

import ReactMarkdown from "react-markdown";
import rehypeKatex from "rehype-katex";
import rehypeRaw from "rehype-raw";
import remarkBreaks from "remark-breaks";
import remarkCjkFriendly from "remark-cjk-friendly";
import remarkGfm from "remark-gfm";
import remarkAlert from "remark-github-blockquote-alert";
import remarkMath from "remark-math";
import remend from "remend";
import { markdownComponents } from "@/components/chat/markdownComponents";
import { measureMarkdownRepair } from "@/lib/metrics";
import { rehypeCitations } from "@/lib/rehypeCitations";
import { rehypeFadeIn } from "@/lib/rehypeFadeIn";
import { useSmoothText } from "@/lib/smoothText";
import "katex/dist/katex.min.css";
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
// regularly emit. We pass a strict allow list to react-markdown
// (`allowElement`) below so scripts / iframes / objects / embeds
// can't sneak through even though they parsed.
const DENIED_HTML_TAGS = new Set(["script", "iframe", "object", "embed", "form"]);

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
  const safe = useMemo(() => {
    const start = performance.now();
    const out = remend(display);
    measureMarkdownRepair(performance.now() - start, display.length, !!streaming);
    return out;
  }, [display, streaming]);

  // Pipeline: rehypeRaw (parse inline HTML) → rehypeCitations (swap
  // `[n]` markers for <sup> badges) → rehypeFadeIn (per-word streaming
  // animation, streamed-only) → rehypeKatex (math). rehypeRaw must
  // come first so subsequent rehype plugins see the expanded tree.
  const rehypePlugins = useMemo(
    () =>
      instant
        ? [rehypeRaw, rehypeCitations, rehypeKatex]
        : [rehypeRaw, rehypeCitations, rehypeFadeIn, rehypeKatex],
    [instant],
  );

  return (
    <div className="md">
      <ReactMarkdown
        remarkPlugins={remarkPlugins}
        rehypePlugins={rehypePlugins}
        components={markdownComponents}
        // Hard-blocklist tags that can execute or break sandbox even if
        // the markdown author wrote them out as raw HTML.
        allowElement={(el) => !DENIED_HTML_TAGS.has(el.tagName)}
      >
        {safe}
      </ReactMarkdown>
    </div>
  );
}
