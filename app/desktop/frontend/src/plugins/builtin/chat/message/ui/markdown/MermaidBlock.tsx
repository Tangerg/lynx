import { useEffect, useMemo, useState } from "react";
import { useDebouncedValue } from "@tanstack/react-pacer";
import { IconButton, LightboxDialog, ShikiCodeBlock } from "@/ui";
import { measureMermaidRender } from "@/lib/metrics";
import { useT } from "@/lib/i18n";
import { useTokenRevision } from "@/lib/appearance";
import { useCopyFeedback } from "@/lib/useCopyFeedback";
import { cn } from "@/lib/classNames";

// `beautiful-mermaid` is heavy (~200KB) and only mounts when an
// agent actually emits a mermaid fence. Cached module promise so
// every subsequent block reuses the same load.
type MermaidRenderer = typeof import("beautiful-mermaid").renderMermaidSVG;
let rendererPromise: Promise<MermaidRenderer> | null = null;
function loadRenderer(): Promise<MermaidRenderer> {
  if (!rendererPromise) {
    rendererPromise = import("beautiful-mermaid").then((m) => m.renderMermaidSVG);
  }
  return rendererPromise;
}

interface Props {
  code: string;
}

type MermaidRenderResult = { status: "loading" | "error" | "rendered"; svg?: string };

interface SettledMermaidRender {
  code: string;
  tokenRevision: number;
  renderer: MermaidRenderer;
  result: MermaidRenderResult;
}

// Resolve token vars to literal hex — beautiful-mermaid bakes the
// values into stroke/fill on the SVG output and browsers won't honor
// raw `var(--x)` text there.
function readThemeColors(_tokenRevision: number) {
  const root = document.documentElement;
  const cs = getComputedStyle(root);
  const grab = (name: string, fallback: string) => cs.getPropertyValue(name).trim() || fallback;
  return {
    fg: grab("--color-text", "#e6e6e6"),
    muted: grab("--color-text-muted", "#9a9a9a"),
    line: grab("--color-text-faint", "#6f6f6f"),
    accent: grab("--color-accent", "#1ed760"),
    surface: grab("--color-surface-2", "#1f1f1f"),
    border: grab("--color-border", "#4d4d4d"),
  };
}

// Debounced 300ms — every parse on an in-progress diagram throws
// (malformed until the closing fence lands), each throw is 30-100ms,
// and stream-reveal feeds chars at ~30 Hz. Until the source settles we
// show a quiet "pending" pre-block; settled + parses → SVG snaps in.
export function MermaidBlock({ code }: Props) {
  const t = useT();
  const fencedCode = useMemo(() => `\`\`\`mermaid\n${code}\n\`\`\``, [code]);
  const { copied, copy } = useCopyFeedback(fencedCode);
  // Mermaid is handed literal colours, so this reads the computed tokens — and
  // needs re-reading whenever the painter rewrites them. The revision is that
  // signal and covers every input, including contrast-derived surface depth.
  const tokenRevision = useTokenRevision();
  const [debouncedCode] = useDebouncedValue(code, { wait: 300 });
  const isSettling = code !== debouncedCode;

  // Lazy-loaded renderer. Stays null until the import resolves; the
  // pending pre-block below covers the gap.
  const [renderer, setRenderer] = useState<MermaidRenderer | null>(null);
  useEffect(() => {
    let alive = true;
    loadRenderer().then((fn) => {
      if (alive) setRenderer(() => fn);
    });
    return () => {
      alive = false;
    };
  }, []);

  const [settledRender, setSettledRender] = useState<SettledMermaidRender | null>(null);
  useEffect(() => {
    if (!renderer || isSettling) return;
    let cancelled = false;
    void Promise.resolve().then(() => {
      let result: MermaidRenderResult;
      try {
        // The revision is an invalidation token for the mutable computed styles
        // read by this adapter, not a colour value in its own right.
        const c = readThemeColors(tokenRevision);
        const start = performance.now();
        const svg = renderer(debouncedCode, {
          transparent: true,
          // `bg` is still required by the type even with transparent:true;
          // beautiful-mermaid uses it for color-mix fallbacks of unset roles.
          bg: c.surface,
          fg: c.fg,
          line: c.line,
          accent: c.accent,
          muted: c.muted,
          surface: c.surface,
          border: c.border,
        });
        measureMermaidRender(performance.now() - start);
        result = { status: "rendered", svg };
      } catch {
        result = { status: "error" };
      }
      if (!cancelled) setSettledRender({ code: debouncedCode, tokenRevision, renderer, result });
    });
    return () => {
      cancelled = true;
    };
  }, [debouncedCode, isSettling, tokenRevision, renderer]);
  const rendered =
    settledRender?.code === debouncedCode &&
    settledRender.tokenRevision === tokenRevision &&
    settledRender.renderer === renderer &&
    !isSettling
      ? settledRender.result
      : { status: "loading" as const };

  const [zoomed, setZoomed] = useState(false);

  if (rendered.status === "rendered") {
    const svg = rendered.svg!;
    return (
      <div
        className="group/mermaid relative isolate my-3.5 min-h-25 w-full rounded-lg border-[0.5px] border-field-strong bg-surface"
        data-markdown-copy="code-block"
        data-markdown-copy-text={fencedCode}
      >
        <div
          role="img"
          aria-label={t("markdown.diagram")}
          tabIndex={-1}
          dir="ltr"
          className="overflow-x-auto p-4 text-center outline-none [&_svg]:h-auto [&_svg]:max-w-full"
          dangerouslySetInnerHTML={{ __html: svg }}
        />
        <div
          className="absolute top-1 right-1 z-1 flex gap-1 opacity-0 transition-opacity group-hover/mermaid:opacity-100 focus-within:opacity-100"
          data-markdown-copy="exclude"
        >
          <LightboxDialog
            open={zoomed}
            onOpenChange={setZoomed}
            title={t("markdown.diagram")}
            className="p-6"
            trigger={
              <IconButton
                icon="maximize"
                size="xs"
                quiet
                title={t("message.mermaid.enlarge")}
                aria-haspopup="dialog"
              />
            }
          >
            <div
              className="[&_svg]:mx-auto [&_svg]:block [&_svg]:max-w-none"
              dangerouslySetInnerHTML={{ __html: svg }}
            />
          </LightboxDialog>
          <IconButton
            icon={copied ? "check" : "copy"}
            size="xs"
            quiet
            onClick={() => void copy()}
            title={t(copied ? "message.mermaid.copied" : "message.mermaid.copy")}
            className={cn(copied && "text-success")}
          />
        </div>
        <span className="sr-only">{t("message.mermaid.source")}</span>
        <pre className="sr-only whitespace-pre-wrap">{code}</pre>
      </div>
    );
  }

  if (rendered.status === "loading") {
    return (
      <div
        role="status"
        aria-label={t("message.mermaid.loading")}
        className="relative my-3.5 grid h-60 min-h-25 w-full place-items-center overflow-hidden rounded-lg border-[0.5px] border-field-strong bg-surface"
      >
        <span aria-hidden="true" className="h-8 w-8 rounded-md bg-surface-3 animate-pulse" />
      </div>
    );
  }

  // A settled parse error is source material, not an endless loader. Reuse the
  // standard code surface so the text stays selectable, wrappable and copyable.
  return <ShikiCodeBlock lang="mermaid" code={code} />;
}
