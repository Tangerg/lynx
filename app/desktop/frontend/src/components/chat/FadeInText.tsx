import { useMemo } from "react";
import ReactMarkdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import { useSmoothText } from "@/lib/smoothText";
import { rehypeFadeIn } from "@/lib/rehypeFadeIn";
import { MermaidBlock } from "@/components/chat/MermaidBlock";
import { ShikiCodeBlock } from "@/components/chat/ShikiCodeBlock";

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

// Pre-process the smoothed prefix so an opening ``` that hasn't yet
// received its closing fence still parses as a code block. Without this,
// react-markdown treats the streaming partial as raw text + literal
// backticks, and the block only "snaps into a code block" once the
// closing ``` arrives — jarring visual flicker.
//
// Also closes unmatched inline backticks and `**` for the same reason:
// the model often emits markers ahead of their closers and we don't want
// the inline styling to jump in mid-flight.
function closeOpenMarkers(s: string): string {
  let out = s;

  // Fenced code blocks. Count ``` occurrences; odd → synthesise a closer.
  const fenceCount = (out.match(/```/g) ?? []).length;
  if (fenceCount % 2 === 1) {
    out = out.endsWith("\n") ? out + "```" : out + "\n```";
  }

  // Inline backticks. Count single ` that aren't part of a ``` (already
  // balanced above). A streaming token like `Refactor \`src/api/auth.ts`
  // hits "unbalanced" between the opener and the closing backtick.
  // Strip fences before counting to avoid double-counting.
  const stripped = out.replace(/```[\s\S]*?```/g, "").replace(/```/g, "");
  const ticks = (stripped.match(/`/g) ?? []).length;
  if (ticks % 2 === 1) {
    out = out + "`";
  }

  // Bold ** pairs. Same idea — strip code first so backticked `**` doesn't
  // count.
  const noCode = out.replace(/`[^`]*`/g, "");
  const stars = (noCode.match(/\*\*/g) ?? []).length;
  if (stars % 2 === 1) {
    out = out + "**";
  }

  return out;
}

// Component overrides for react-markdown.
//
// - `pre`: unwrap. Our `code` override below handles the entire block-
//   level rendering (Shiki / Mermaid / plain fallback) and emits its
//   own outer container — keeping the default `<pre>` would double-wrap.
// - `code`: route by language tag. Mermaid blocks render as diagrams,
//   everything else with a recognised `language-X` className goes
//   through Shiki. Inline code (no language- prefix) stays as a plain
//   `<code>` so it can sit in the text flow with .fade-in alongside it.
const components: Components = {
  pre({ children }) {
    return <>{children}</>;
  },
  code({ className, children, ...rest }) {
    const cls = String(className ?? "");
    const match = /language-([\w+-]+)/.exec(cls);
    if (!match) {
      return (
        <code className={cls} {...rest}>
          {children}
        </code>
      );
    }
    const lang = match[1].toLowerCase();
    const codeStr = String(children ?? "").replace(/\n$/, "");
    if (lang === "mermaid") return <MermaidBlock code={codeStr} />;
    return <ShikiCodeBlock lang={lang} code={codeStr} />;
  },
};

const remarkPlugins = [remarkGfm];

// FadeInText renders streaming text as full Markdown with per-word
// fade-in.
//
// Pipeline:
//   raw text
//     → useSmoothText (paces the reveal at ~40-160 chars/sec)
//     → closeOpenMarkers (synthesises ```, `, ** closers mid-stream)
//     → react-markdown + remark-gfm (tables, task lists, strikethrough)
//     → rehype: rehypeFadeIn wraps non-code text nodes in
//       <span class="fade-in">word</span>
//     → component map: code blocks → Shiki, mermaid blocks → SVG
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
export function FadeInText({ text, streaming, instant }: Props) {
  // Hook must run unconditionally; when `instant`, pass enabled=false so
  // it jumps to full length and idles.
  const smoothed = useSmoothText(text, !instant && !!streaming);
  const display = instant ? text : smoothed;
  const safe = useMemo(() => closeOpenMarkers(display), [display]);

  const rehypePlugins = useMemo(
    () => (instant ? [] : [rehypeFadeIn]),
    [instant],
  );

  return (
    <div className="md">
      <ReactMarkdown
        remarkPlugins={remarkPlugins}
        rehypePlugins={rehypePlugins}
        components={components}
      >
        {safe}
      </ReactMarkdown>
    </div>
  );
}
