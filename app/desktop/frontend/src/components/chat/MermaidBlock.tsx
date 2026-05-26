import { AnimatePresence, motion } from "motion/react";
import { useEffect, useMemo, useState } from "react";
import { useDebounce } from "use-debounce";
import { popIn, swift } from "@/lib/motion";
import { useThemeStore } from "@/state/themeStore";

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
// and smooth-text feeds chars at ~30 Hz. Until the source settles we
// show a quiet "pending" pre-block; settled + parses → SVG snaps in.
export function MermaidBlock({ code }: Props) {
  // theme + accent feed into readThemeColors() below via deps so the
  // diagram re-paints when the palette switches.
  const theme = useThemeStore((s) => s.theme);
  const accent = useThemeStore((s) => s.accent);
  const [debouncedCode] = useDebounce(code, 300);
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

  // Render swallows errors via the pending pre-block fallback; underscore
  // tells ESLint the destructure is intentional.
  const { svg, error: _error } = useMemo(() => {
    if (!renderer || isSettling) {
      return { svg: null, error: null as Error | null };
    }
    try {
      const c = readThemeColors();
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

  // Esc closes the lightbox — only bind the listener while it's open so
  // we don't compete with other Escape handlers (composer, palette).
  useEffect(() => {
    if (!zoomed) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") {
        e.preventDefault();
        setZoomed(false);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [zoomed]);

  if (svg) {
    return (
      <>
        <div
          role="button"
          tabIndex={0}
          title="Click to enlarge"
          onClick={() => setZoomed(true)}
          onKeyDown={(e) => {
            if (e.key === "Enter" || e.key === " ") {
              e.preventDefault();
              setZoomed(true);
            }
          }}
          // Inline SVG sizes itself; the wrapper provides chrome + zoom
          // affordance. `[&_svg]:` reaches the SVG that
          // dangerouslySetInnerHTML drops in (we can't put utilities on it
          // directly).
          className="my-3.5 cursor-zoom-in overflow-x-auto rounded-lg border border-[color-mix(in_srgb,var(--color-text)_10%,transparent)] bg-[color-mix(in_srgb,var(--color-text)_3%,transparent)] p-4 text-center transition-colors duration-150 hover:border-[color-mix(in_srgb,var(--color-accent)_30%,transparent)] [&_svg]:max-w-full [&_svg]:h-auto"
          dangerouslySetInnerHTML={{ __html: svg }}
        />
        <AnimatePresence>
          {zoomed && (
            <motion.div
              initial={{ opacity: 0 }}
              animate={{ opacity: 1 }}
              exit={{ opacity: 0 }}
              transition={swift}
              onClick={() => setZoomed(false)}
              role="dialog"
              aria-modal="true"
              // Light theme keeps the backdrop quieter — already-light
              // page bg + a 60% black wash reads as a flat grey haze.
              className="fixed inset-0 z-[200] grid cursor-zoom-out place-items-center bg-black/60 light:bg-black/25 backdrop-blur-[8px] p-10"
            >
              <motion.div
                {...popIn}
                onClick={(e) => e.stopPropagation()}
                // Frame: a Panel-style card that pops the SVG out of the
                // backdrop. SVG renders at native scale (max-width: none)
                // so the user gets the full readable diagram.
                className="max-h-[90vh] max-w-[min(1400px,95vw)] overflow-auto rounded-xl border border-line-soft bg-surface p-6 shadow-lg cursor-default [&_svg]:block [&_svg]:mx-auto [&_svg]:max-w-none"
                dangerouslySetInnerHTML={{ __html: svg }}
              />
            </motion.div>
          )}
        </AnimatePresence>
      </>
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
