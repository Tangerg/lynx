import type { Components } from "react-markdown";
import * as Tooltip from "@radix-ui/react-tooltip";
import { useEffect, useRef } from "react";
import { useCitations } from "@/components/chat/CitationContext";
import { HtmlArtifact } from "@/components/chat/HtmlArtifact";
import { MermaidBlock } from "@/components/chat/MermaidBlock";
import { ShikiCodeBlock } from "@/components/chat/ShikiCodeBlock";

// Citation badge — `[n]` markers in markdown turn into <sup
// data-citation="n">. This component reads the per-message
// CitationContext to find the matching source and wraps the marker in
// a Radix Tooltip with the source's title + snippet.
function CitationBadge({ n, label }: { n: number; label: string }) {
  const citations = useCitations();
  const source = citations.find((c) => c.index === n);

  // No source matched (e.g. agent wrote `[3]` but only 2 results in
  // search block). Render the marker as plain text — no tooltip.
  if (!source) {
    return (
      <sup className="cite-marker text-fg-faint" data-citation={n}>
        {label}
      </sup>
    );
  }

  return (
    <Tooltip.Provider delayDuration={200}>
      <Tooltip.Root>
        <Tooltip.Trigger asChild>
          <sup
            className="cite-marker rounded-sm bg-surface-2 px-[3px] py-px font-mono text-[10px] font-semibold text-fg-soft cursor-help hover:bg-accent hover:text-on-accent transition-colors"
            data-citation={n}
          >
            {label}
          </sup>
        </Tooltip.Trigger>
        <Tooltip.Portal>
          <Tooltip.Content
            side="top"
            sideOffset={6}
            className="z-50 max-w-[360px] rounded-md border border-line bg-surface px-3 py-2 shadow-lg"
          >
            <div className="text-[11px] font-mono text-fg-faint tabular-nums">{source.domain}</div>
            <div className="mt-0.5 text-[12.5px] font-semibold text-fg leading-snug">
              {source.title}
            </div>
            <div className="mt-1 text-[11.5px] text-fg-muted leading-snug line-clamp-3">
              {source.snippet}
            </div>
          </Tooltip.Content>
        </Tooltip.Portal>
      </Tooltip.Root>
    </Tooltip.Provider>
  );
}

// Render an agent-emitted <style> block inside a Shadow DOM so its
// rules don't bleed into the host app. The shadow root is empty + only
// hosts a <style> with the original text — no markup, no DOM, no
// observable side-effects on the surrounding chat. This lets agents
// write `<style>p { color: red }</style>` snippets in their output
// without nuking our typography.
function ShadowStyleBlock({ children }: { children?: React.ReactNode }) {
  const hostRef = useRef<HTMLSpanElement>(null);
  const css = typeof children === "string" ? children : "";

  useEffect(() => {
    const host = hostRef.current;
    if (!host) return;
    const shadow = host.shadowRoot ?? host.attachShadow({ mode: "open" });
    shadow.innerHTML = `<style>${css}</style>`;
  }, [css]);

  // Host stays display:none — it's only a Shadow DOM mount point.
  return <span ref={hostRef} style={{ display: "none" }} aria-hidden="true" />;
}

// react-markdown component overrides. `pre` unwraps because `code`
// below emits its own block container (would otherwise double-wrap).
// `a` opens external links in the OS browser — clicking a link inside
// the Wails WebView would otherwise navigate the chrome-less window.
export const markdownComponents: Components = {
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
    if (lang === "html" || lang === "htm") return <HtmlArtifact code={codeStr} />;
    return <ShikiCodeBlock lang={lang} code={codeStr} />;
  },
  table({ children, ...rest }) {
    return (
      <div className="md-table-wrap">
        <table {...rest}>{children}</table>
      </div>
    );
  },
  a({ href, children, ...rest }) {
    return (
      <a href={href} target="_blank" rel="noopener noreferrer" {...rest}>
        {children}
      </a>
    );
  },
  // <style> inside agent output → Shadow DOM, never reaches the
  // global stylesheet. Pairs with the rehype-raw plugin that lets the
  // tag through in the first place.
  style({ children }) {
    return <ShadowStyleBlock>{children}</ShadowStyleBlock>;
  },
  // <sup data-citation="n"> is what rehypeCitations emits for each `[n]`
  // marker in source markdown. Defer to CitationBadge so the same tag
  // stays a no-op for any unrelated `<sup>` the user wrote by hand.
  sup({ children, ...rest }) {
    const ds = (rest as { "data-citation"?: string })["data-citation"];
    if (ds) {
      const label = typeof children === "string" ? children : `[${ds}]`;
      return <CitationBadge n={Number(ds)} label={label} />;
    }
    return <sup {...rest}>{children}</sup>;
  },
};
