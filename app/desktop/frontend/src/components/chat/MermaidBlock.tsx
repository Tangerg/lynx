import { useMemo } from "react";
import { renderMermaidSVG } from "beautiful-mermaid";
import { useDebouncedValue } from "@/lib/useDebouncedValue";

type Props = {
  code: string;
};

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
      const out = renderMermaidSVG(debouncedCode, { transparent: true });
      return { svg: out, error: null as Error | null };
    } catch (err) {
      return {
        svg: null,
        error: err instanceof Error ? err : new Error(String(err)),
      };
    }
  }, [debouncedCode, isSettling]);

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
