import { useEffect, useMemo, useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import { renderMermaidSVG } from "beautiful-mermaid";
import { useDebounce } from "use-debounce";
import { useThemeStore } from "@/state/themeStore";
import { popIn, swift } from "@/lib/motion";

type Props = {
  code: string;
};

// Pull the resolved hex values for our theme tokens out of CSS so the
// diagram follows light/dark switches. We can't pass `var(--x)` directly —
// the SVG ends up with literal text "var(--x)" baked into stroke/fill
// attributes that browsers then refuse to honor on inline SVGs.
function readThemeColors() {
  const root = document.documentElement;
  const cs = getComputedStyle(root);
  const grab = (name: string, fallback: string) =>
    cs.getPropertyValue(name).trim() || fallback;
  return {
    fg:      grab("--color-text",       "#e6e6e6"),
    muted:   grab("--color-text-muted", "#9a9a9a"),
    line:    grab("--color-text-faint", "#6f6f6f"),
    accent:  grab("--color-accent",     "#1ed760"),
    surface: grab("--color-surface-2",  "#1f1f1f"),
    border:  grab("--color-border",     "#4d4d4d"),
  };
}

// MermaidBlock — beautiful-mermaid's synchronous SVG renderer, gated by
// a debounce.
//
// Why we debounce harder than Shiki: every parse attempt on an
// in-progress diagram throws (the source is malformed until the closing
// fence and the full directive land). Each throw is 30-100ms of CPU,
// and smooth-text feeds new chars at ~30 Hz — running the parser on
// every delta freezes the chat. We let `code` settle for 300ms before
// attempting a render; until then we show the live source in a quiet
// "pending" pre-block. Once the source stabilises and parses, the SVG
// snaps in.
export function MermaidBlock({ code }: Props) {
  // Re-render whenever theme/accent changes so the diagram re-paints
  // against the new palette. We don't need the values themselves — they
  // come from getComputedStyle at render time — just a dependency to
  // trigger useMemo invalidation.
  const theme  = useThemeStore((s) => s.theme);
  const accent = useThemeStore((s) => s.accent);
  const [debouncedCode] = useDebounce(code, 300);
  const isSettling = code !== debouncedCode;

  const { svg, error } = useMemo(() => {
    // Don't even attempt while still streaming — saves the parse cost
    // entirely. Once the value settles, useMemo recomputes against the
    // settled string and either gives us SVG or a real parse failure.
    if (isSettling) {
      return { svg: null, error: null as Error | null };
    }
    try {
      const c = readThemeColors();
      const out = renderMermaidSVG(debouncedCode, {
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
  }, [debouncedCode, isSettling, theme, accent]);

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
