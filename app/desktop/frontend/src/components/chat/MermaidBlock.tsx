import { useMemo } from "react";
import { renderMermaidSVG } from "beautiful-mermaid";
import { useDebouncedValue } from "@/lib/useDebouncedValue";
import { useUIStore } from "@/state/uiStore";

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
  const theme  = useUIStore((s) => s.theme);
  const accent = useUIStore((s) => s.accent);
  const debouncedCode = useDebouncedValue(code, 300);
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

  if (svg) {
    return <div className="mermaid-block" dangerouslySetInnerHTML={{ __html: svg }} />;
  }

  // Streaming or genuinely-broken source. Show the in-progress source as
  // quiet preformatted text — readable, no scary error chrome. Once the
  // closing ``` arrives and the diagram parses cleanly we swap to SVG;
  // the visual transition reads as progressive disclosure rather than a
  // flicker between error / success states.
  return (
    <pre className="mermaid-block-pending">
      <code>{code}</code>
    </pre>
  );
}
