import type { Components } from "react-markdown";
import { useEffect, useRef } from "react";
import { RichTooltip, ShikiCodeBlock } from "@/components/common";
import { useCitations } from "../CitationContext";
import { FileRefLink } from "../FileRefLink";
import { HtmlArtifact } from "./HtmlArtifact";
import { MermaidBlock } from "./MermaidBlock";

// Per-message citation lookup. CitationContext is scoped to the
// owning message so two messages with [1] markers don't collide.
function CitationBadge({ n, label }: { n: number; label: string }) {
  const citations = useCitations();
  const source = citations.find((c) => c.index === n);

  // Marker without a matching source (e.g. agent wrote [3] but only
  // 2 results in search block) renders as plain text — no tooltip.
  if (!source) {
    return (
      <sup className="cite-marker text-fg-faint" data-citation={n}>
        {label}
      </sup>
    );
  }

  return (
    <RichTooltip
      delay={200}
      side="top"
      sideOffset={6}
      className="max-w-[360px] px-3 py-2"
      trigger={
        <sup
          className="cite-marker rounded-sm bg-surface-2 px-[3px] py-px font-mono text-[10px] font-semibold text-fg-soft cursor-help hover:bg-accent hover:text-on-accent transition-colors"
          data-citation={n}
        >
          {label}
        </sup>
      }
    >
      <div className="text-[11px] font-mono text-fg-faint">{source.domain}</div>
      <div className="mt-0.5 text-[12.5px] font-semibold text-fg leading-snug">{source.title}</div>
      <div className="mt-1 text-[11.5px] text-fg-muted leading-snug line-clamp-3">
        {source.snippet}
      </div>
    </RichTooltip>
  );
}

// Agent-emitted <style> blocks go through a Shadow DOM so their
// rules can't escape and clobber the host stylesheet. Pairs with
// rehype-raw, which is what lets the tag survive sanitization in
// the first place.
function ShadowStyleBlock({ children }: { children?: React.ReactNode }) {
  const hostRef = useRef<HTMLSpanElement>(null);
  const css = typeof children === "string" ? children : "";

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;
    const shadow = host.shadowRoot ?? host.attachShadow({ mode: "open" });
    shadow.innerHTML = `<style>${css}</style>`;
  }, [css]);

  return <span ref={hostRef} style={{ display: "none" }} aria-hidden="true" />;
}

// `pre` unwraps because the `code` override below emits its own block
// container. `a` forces target=_blank because a click inside the
// Wails WebView would otherwise navigate the chrome-less window away
// from the app.
export const markdownComponents: Components = {
  pre({ children }) {
    return <>{children}</>;
  },
  code({ className, children }) {
    const cls = String(className ?? "");
    const match = /language-([\w+-]+)/.exec(cls);
    if (!match) {
      // Don't spread the rest props — react-markdown's passNode puts the hast
      // `node` in there, which would leak onto the DOM as node="[object Object]".
      return <code className={cls}>{children}</code>;
    }
    // Regex has a capture group, so match[1] is defined when match is.
    const lang = match[1]!.toLowerCase();
    const codeStr = String(children ?? "").replace(/\n$/, "");
    if (lang === "mermaid") return <MermaidBlock code={codeStr} />;
    if (lang === "html" || lang === "htm") return <HtmlArtifact code={codeStr} />;
    return <ShikiCodeBlock lang={lang} code={codeStr} />;
  },
  table({ children }) {
    // No rest spread — keep the hast `node` off the DOM (see `code`).
    return (
      <div className="md-table-wrap">
        <table>{children}</table>
      </div>
    );
  },
  a({ href, title, children, ...rest }) {
    // A `data-file-ref` anchor is emitted by rehypeFileRefs (not a real link) —
    // render it as a FileRefLink that opens the file viewer instead of
    // navigating. `data-file-line` is "0" / absent when no line was parsed.
    const r = rest as { "data-file-ref"?: string; "data-file-line"?: string };
    if (r["data-file-ref"]) {
      return <FileRefLink path={r["data-file-ref"]} line={Number(r["data-file-line"]) || 0} />;
    }
    // Forward only real anchor attrs (href/title); the rest carries the hast
    // `node`, which must not reach the DOM.
    return (
      <a href={href} title={title} target="_blank" rel="noopener noreferrer">
        {children}
      </a>
    );
  },
  style({ children }) {
    return <ShadowStyleBlock>{children}</ShadowStyleBlock>;
  },
  // Only `<sup>` carrying `data-citation` (emitted by rehypeCitations)
  // becomes a CitationBadge; any other `<sup>` the user wrote by hand
  // passes through unchanged.
  sup({ children, ...rest }) {
    const ds = (rest as { "data-citation"?: string })["data-citation"];
    const n = Number(ds);
    // Only the rehypeCitations-emitted numeric data-citation becomes a badge; a
    // hand-authored `<sup data-citation="abc">` (n=NaN) falls through to plain.
    if (ds && Number.isInteger(n)) {
      const label = typeof children === "string" ? children : `[${ds}]`;
      return <CitationBadge n={n} label={label} />;
    }
    // No rest spread — keep the hast `node` off the DOM (see `code`).
    return <sup>{children}</sup>;
  },
};
