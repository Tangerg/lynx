import type { Components } from "react-markdown";
import * as Tooltip from "@radix-ui/react-tooltip";
import { useEffect, useRef } from "react";
import { useCitations } from "@/components/chat/CitationContext";
import { HtmlArtifact } from "@/components/chat/HtmlArtifact";
import { MermaidBlock } from "@/components/chat/MermaidBlock";
import { ShikiCodeBlock } from "@/components/chat/ShikiCodeBlock";

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

  // Rich tooltip content (3-line card) is too custom for the shared
  // `<Tooltip>` wrapper, so we use Radix primitives directly — but the
  // Provider is mounted once at the app root (PluginProvider), so no
  // local Provider is needed here.
  return (
    <Tooltip.Root delayDuration={200}>
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
    // Regex has a capture group, so match[1] is defined when match is.
    const lang = match[1]!.toLowerCase();
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
  style({ children }) {
    return <ShadowStyleBlock>{children}</ShadowStyleBlock>;
  },
  // Only `<sup>` carrying `data-citation` (emitted by rehypeCitations)
  // becomes a CitationBadge; any other `<sup>` the user wrote by hand
  // passes through unchanged.
  sup({ children, ...rest }) {
    const ds = (rest as { "data-citation"?: string })["data-citation"];
    if (ds) {
      const label = typeof children === "string" ? children : `[${ds}]`;
      return <CitationBadge n={Number(ds)} label={label} />;
    }
    return <sup {...rest}>{children}</sup>;
  },
};
