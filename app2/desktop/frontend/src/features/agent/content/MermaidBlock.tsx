import { useEffect, useId, useRef, useState } from "react";

import { useShellPreferences } from "../../preferences/ShellPreferences";
import { CodeBlock } from "./CodeBlock";

let mermaidModule: Promise<typeof import("mermaid")> | undefined;
let mermaidSequence = 0;
let mermaidRenderQueue = Promise.resolve();

export function MermaidBlock({ source }: { source: string }) {
  const { resolvedTheme } = useShellPreferences();
  const container = useRef<HTMLDivElement>(null);
  const identity = `lyra-diagram-${useId().replaceAll(":", "")}`;
  const [visible, setVisible] = useState(false);
  const [markup, setMarkup] = useState<string>();
  const [error, setError] = useState(false);

  useEffect(() => {
    const element = container.current;
    if (element === null || visible) return;
    if (!("IntersectionObserver" in window)) {
      setVisible(true);
      return;
    }
    const observer = new IntersectionObserver(
      ([entry]) => {
        if (!entry?.isIntersecting) return;
        setVisible(true);
        observer.disconnect();
      },
      { rootMargin: "240px" },
    );
    observer.observe(element);
    return () => observer.disconnect();
  }, [visible]);

  useEffect(() => {
    if (!visible) return;
    let current = true;
    setMarkup(undefined);
    setError(false);
    void renderMermaid(identity, source, resolvedTheme).then((svg) => {
      if (!current) return;
      if (svg === undefined) {
        setError(true);
        return;
      }
      setMarkup(svg);
    });
    return () => {
      current = false;
    };
  }, [identity, resolvedTheme, source, visible]);

  return (
    <section
      className="mermaid-block"
      ref={container}
      aria-busy={visible && !markup && !error}
    >
      <header>
        <span>Diagram</span>
      </header>
      {markup ? (
        <div
          className="mermaid-canvas"
          dangerouslySetInnerHTML={{ __html: markup }}
        />
      ) : error ? (
        <div>
          <p role="status">This diagram could not be rendered.</p>
          <CodeBlock code={source} language="mermaid" />
        </div>
      ) : (
        <div className="mermaid-placeholder" role="status">
          {visible ? "Rendering diagram…" : "Diagram loads when visible"}
        </div>
      )}
    </section>
  );
}

async function renderMermaid(
  identity: string,
  source: string,
  theme: "linen" | "graphite",
) {
  const renderIdentity = `${identity}-${++mermaidSequence}`;
  const result = mermaidRenderQueue.then(() =>
    renderMermaidNow(renderIdentity, source, theme),
  );
  mermaidRenderQueue = result.then(
    () => undefined,
    () => undefined,
  );
  return result;
}

async function renderMermaidNow(
  identity: string,
  source: string,
  theme: "linen" | "graphite",
) {
  try {
    mermaidModule ??= import("mermaid");
    const { default: mermaid } = await mermaidModule;
    mermaid.initialize({
      startOnLoad: false,
      securityLevel: "strict",
      suppressErrorRendering: true,
      theme: theme === "graphite" ? "dark" : "neutral",
      fontFamily:
        'Inter, ui-sans-serif, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
    });
    const { svg } = await mermaid.render(identity, source);
    const { default: DOMPurify } = await import("dompurify");
    return DOMPurify.sanitize(svg, {
      USE_PROFILES: { svg: true, svgFilters: true },
    });
  } catch {
    return undefined;
  }
}
