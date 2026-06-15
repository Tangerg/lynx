import * as Dialog from "@radix-ui/react-dialog";
import { useEffect, useMemo, useState } from "react";
import { useDebounce } from "use-debounce";
import { measureMermaidRender } from "@/lib/metrics";
import { useT } from "@/lib/i18n";
import { useUiStore } from "@/state/uiStore";

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
function readThemeColors() {
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
  // theme + accent feed into readThemeColors() below via deps so the
  // diagram re-paints when the palette switches.
  const t = useT();
  const theme = useUiStore((s) => s.theme);
  const accent = useUiStore((s) => s.accent);
  const [debouncedCode] = useDebounce(code, 300);
  const isSettling = code !== debouncedCode;

  // Lazy-loaded renderer. Stays null until the import resolves; the
  // pending pre-block below covers the gap.
  const [renderer, setRenderer] = useState<MermaidRenderer | null>(null);
  useEffect(() => {
    let alive = true;
    loadRenderer().then((fn) => {
      if (alive) setRenderer(fn);
    });
    return () => {
      alive = false;
    };
  }, []);

  // Render swallows errors via the pending pre-block fallback; underscore
  // tells ESLint the destructure is intentional.
  const { svg, error: _error } = useMemo(() => {
    if (!renderer || isSettling) {
      return { svg: null, error: null as Error | null };
    }
    try {
      const c = readThemeColors();
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
      return { svg: out, error: null as Error | null };
    } catch (err) {
      return {
        svg: null,
        error: err instanceof Error ? err : new Error(String(err)),
      };
    }
    // theme + accent feel "unnecessary" to the static analyser because
    // useMemo doesn't read them directly, but readThemeColors() pulls
    // them from getComputedStyle at call time. Pinning them in deps
    // re-triggers the memo when the user switches palette so the SVG
    // re-paints with the new tokens.
    // eslint-disable-next-line react/exhaustive-deps
  }, [debouncedCode, isSettling, theme, accent, renderer]);

  const [zoomed, setZoomed] = useState(false);

  if (svg) {
    return (
      <Dialog.Root open={zoomed} onOpenChange={setZoomed}>
        <Dialog.Trigger
          type="button"
          aria-label={t("message.mermaid.enlarge")}
          title={t("message.mermaid.enlargeHint")}
          // Inline SVG sizes itself; the wrapper provides chrome + zoom
          // affordance. `[&_svg]:` reaches the SVG that
          // dangerouslySetInnerHTML drops in (we can't put utilities on it
          // directly).
          className="my-3.5 w-full cursor-zoom-in overflow-x-auto rounded-lg border border-[color-mix(in_srgb,var(--color-text)_10%,transparent)] bg-[color-mix(in_srgb,var(--color-text)_3%,transparent)] p-4 text-center transition-colors duration-150 hover:border-[color-mix(in_srgb,var(--color-accent)_30%,transparent)] [&_svg]:h-auto [&_svg]:max-w-full"
          dangerouslySetInnerHTML={{ __html: svg }}
        />
        <Dialog.Portal>
          {/* Backdrop — quieter on light (already-light page + a wash).
              cursor-zoom-out signals click-to-close (Radix closes on the
              outside-click + Esc + traps focus + locks scroll). */}
          <Dialog.Overlay className="fixed inset-0 z-[200] cursor-zoom-out bg-black/60 backdrop-blur-[8px] light:bg-black/25" />
          {/* The framed SVG, centered via inset-0 + m-auto (no transform, so it
              doesn't fight the rise-in keyframe). Native scale = fully readable;
              pop-in on open via the shared rise-in keyframe. */}
          <Dialog.Content
            aria-describedby={undefined}
            className="fixed inset-0 z-[201] m-auto h-fit w-fit max-h-[90vh] max-w-[min(1400px,95vw)] overflow-auto rounded-xl border border-line-soft bg-surface p-6 shadow-lg outline-none data-[state=open]:animate-rise-in"
          >
            <Dialog.Title className="sr-only">Diagram</Dialog.Title>
            <div
              className="[&_svg]:mx-auto [&_svg]:block [&_svg]:max-w-none"
              dangerouslySetInnerHTML={{ __html: svg }}
            />
          </Dialog.Content>
        </Dialog.Portal>
      </Dialog.Root>
    );
  }

  // Streaming or genuinely-broken source. Show the in-progress source as
  // quiet preformatted text — readable, no scary error chrome. Once the
  // closing ``` arrives and the diagram parses cleanly we swap to SVG;
  // the visual transition reads as progressive disclosure rather than a
  // flicker between error / success states.
  return (
    <pre className="my-3.5 overflow-x-auto whitespace-pre rounded-lg border border-dashed border-[color-mix(in_srgb,var(--color-text)_14%,transparent)] bg-[color-mix(in_srgb,var(--color-text)_2%,transparent)] px-3.5 py-3 font-mono text-[12px] leading-[1.55] text-fg-faint">
      <code>{code}</code>
    </pre>
  );
}
