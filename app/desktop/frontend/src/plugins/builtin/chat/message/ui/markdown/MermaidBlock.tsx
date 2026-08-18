import { useEffect, useMemo, useState } from "react";
import { useDebouncedValue } from "@tanstack/react-pacer";
import { LightboxDialog, Pressable } from "@/ui";
import { measureMermaidRender } from "@/lib/metrics";
import { useT } from "@/lib/i18n";
import { useTokenRevision } from "@/lib/appearance";

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

  // Rendering errors use the same quiet source fallback as an in-progress
  // diagram. There is no separate error state because the UI has no separate
  // error presentation or recovery action.
  const svg = useMemo(() => {
    if (!renderer || isSettling) {
      return null;
    }
    try {
      // The revision is an invalidation token for the mutable computed styles
      // read by this adapter, not a colour value in its own right.
      const c = readThemeColors(tokenRevision);
      const start = performance.now();
      const out = renderer(debouncedCode, {
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
      return out;
    } catch {
      return null;
    }
  }, [debouncedCode, isSettling, tokenRevision, renderer]);

  const [zoomed, setZoomed] = useState(false);

  if (svg) {
    return (
      <LightboxDialog
        open={zoomed}
        onOpenChange={setZoomed}
        title={t("markdown.diagram")}
        className="p-6"
        trigger={
          <Pressable
            type="button"
            aria-label={t("message.mermaid.enlarge")}
            title={t("message.mermaid.enlargeHint")}
            // Inline SVG sizes itself; the wrapper provides chrome + zoom
            // affordance. `[&_svg]:` reaches the SVG that
            // dangerouslySetInnerHTML drops in (we can't put utilities on it
            // directly).
            className="my-3.5 w-full cursor-zoom-in overflow-x-auto rounded-lg border-[0.5px] border-field-strong bg-surface p-4 text-center transition-colors duration-[var(--dur-fast)] hover:border-accent-badge [&_svg]:h-auto [&_svg]:max-w-full"
            dangerouslySetInnerHTML={{ __html: svg }}
          />
        }
      >
        <div
          className="[&_svg]:mx-auto [&_svg]:block [&_svg]:max-w-none"
          dangerouslySetInnerHTML={{ __html: svg }}
        />
      </LightboxDialog>
    );
  }

  // Streaming or genuinely-broken source. Show the in-progress source as
  // quiet preformatted text — readable, no scary error chrome. Once the
  // closing ``` arrives and the diagram parses cleanly we swap to SVG;
  // the visual transition reads as progressive disclosure rather than a
  // flicker between error / success states.
  return (
    <pre className="my-3.5 overflow-x-auto whitespace-pre rounded-lg border-[0.5px] border-dashed border-field-strong bg-surface px-3.5 py-3 font-mono text-ui-md leading-body text-fg-faint">
      <code>{code}</code>
    </pre>
  );
}
