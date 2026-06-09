import { useEffect, useMemo, useRef, useState } from "react";
import { useDebounce } from "use-debounce";
import { Icon } from "@/components/common";
import { measureShikiHighlight } from "@/lib/metrics";
import { getHighlighter, resolveLang } from "@/lib/markdown/shiki";
import { getCachedHighlight, setCachedHighlight } from "@/lib/markdown/shikiCache";
import { cn } from "@/lib/utils";
import { resolveScheme } from "@/plugins/sdk";
import { useUiStore } from "@/state/uiStore";

interface Props {
  lang: string;
  code: string;
  /**
   * Optional filename to display in the header. When set, the lang pill
   * sits on the left and the filename takes the centre column.
   */
  file?: string;
}

// We debounce `code` so the Shiki tokenizer (3-10ms per pass) doesn't
// run on every stream-reveal delta during streaming. While it's settling,
// raw code shows in a <pre> fallback. Blocks longer than this auto-fold
// once the stream finishes.
const FOLD_LINE_THRESHOLD = 24;

export function ShikiCodeBlock({ lang, code, file }: Props) {
  const themeId = useUiStore((s) => s.theme);
  // resolveScheme via the registry so third-party light themes ("solarized-
  // light" etc.) also pick the right shiki preset, not just id === "light".
  const scheme = resolveScheme(themeId);
  const shikiTheme = scheme === "light" ? "github-light" : "github-dark";

  const [debouncedCode] = useDebounce(code, 120);
  const isSettling = code !== debouncedCode;

  // Seed from cache synchronously so a re-mount (scroll away/back, theme
  // toggle returning to a prior theme, MarkdownBlock memo invalidation
  // on a long history) skips both the async highlighter resolution and
  // the tokenizer call. Cache key is (lang, theme, exact-code).
  const [html, setHtml] = useState<string | null>(
    () => getCachedHighlight(lang, shikiTheme, debouncedCode) ?? null,
  );
  const [copied, setCopied] = useState(false);
  const [expanded, setExpanded] = useState(false);

  const lineCount = useMemo(() => code.split("\n").length, [code]);
  // Don't fold while the stream is in flight — collapsing a growing
  // block hides the agent's progress.
  const folded = !expanded && !isSettling && lineCount > FOLD_LINE_THRESHOLD;

  useEffect(() => {
    // Fast path — cache hit means we never wake the async highlighter.
    const cached = getCachedHighlight(lang, shikiTheme, debouncedCode);
    if (cached !== undefined) {
      setHtml(cached);
      return;
    }

    let cancelled = false;
    getHighlighter()
      .then((h) => {
        if (cancelled) return;
        try {
          const resolvedLang = resolveLang(h, lang);
          const start = performance.now();
          const out = h.codeToHtml(debouncedCode, {
            lang: resolvedLang,
            theme: shikiTheme,
          });
          measureShikiHighlight(performance.now() - start, resolvedLang);
          setCachedHighlight(lang, shikiTheme, debouncedCode, out);
          setHtml(out);
        } catch {
          setHtml(null);
        }
      })
      .catch(() => {
        if (!cancelled) setHtml(null);
      });
    return () => {
      cancelled = true;
    };
  }, [lang, debouncedCode, shikiTheme]);

  // setTimeout id for the "Copied" → idle flip. Tracked so we can clear
  // it on unmount (otherwise a fast-mount/unmount or re-copy stacks
  // timers and fires setState on an unmounted component).
  const copyTimerRef = useRef<number | null>(null);
  useEffect(
    () => () => {
      if (copyTimerRef.current !== null) window.clearTimeout(copyTimerRef.current);
    },
    [],
  );

  const onCopy = () => {
    try {
      navigator.clipboard?.writeText(code);
    } catch {
      /* ignore */
    }
    setCopied(true);
    if (copyTimerRef.current !== null) window.clearTimeout(copyTimerRef.current);
    copyTimerRef.current = window.setTimeout(() => {
      setCopied(false);
      copyTimerRef.current = null;
    }, 1500);
  };

  // Streaming → raw <pre> fallback; settled → swap to highlighted.
  // Falls back indefinitely if the highlighter never resolves.
  const showHighlighted = !isSettling && html !== null;

  return (
    // `shiki-block` is a CSS hook for markdown.css rules that style the
    // `<pre class="shiki">` + child `<code>` Shiki emits as a string.
    <div
      className={cn(
        "shiki-block group/code my-3.5 overflow-hidden rounded-lg font-mono text-[13px]",
        "border border-[color-mix(in_srgb,var(--color-text)_10%,transparent)]",
        "bg-[color-mix(in_srgb,var(--color-text)_3%,transparent)]",
        folded && "folded",
      )}
    >
      <div className="grid grid-cols-[auto_1fr_auto] items-center gap-2.5 border-b border-[color-mix(in_srgb,var(--color-text)_7%,transparent)] pl-3.5 pr-3 py-2">
        <span className="font-sans text-[10px] font-semibold text-fg-faint tracking-normal normal-case">
          {lang || "text"}
        </span>
        {file ? (
          <span className="truncate font-mono text-[11.5px] text-fg-muted">{file}</span>
        ) : (
          <span aria-hidden="true" />
        )}
        <button
          type="button"
          onClick={onCopy}
          title={copied ? "Copied" : "Copy code"}
          className="inline-flex items-center gap-1 rounded-md border-0 bg-transparent px-2 py-1 font-mono text-[11px] font-semibold text-fg-faint transition-[opacity,color,background] duration-150 opacity-60 group-hover/code:opacity-100 hover:!text-fg hover:bg-[color-mix(in_srgb,var(--color-text)_8%,transparent)]"
        >
          <Icon name={copied ? "check" : "copy"} size={11} />
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
      {folded ? (
        <button
          type="button"
          onClick={() => setExpanded(true)}
          title="Expand code block"
          className={FOLD_TOGGLE}
        >
          <Icon name="code" size={12} />
          <span>Show {lineCount} lines</span>
        </button>
      ) : (
        <>
          {showHighlighted ? (
            <div className="shiki-body" dangerouslySetInnerHTML={{ __html: html! }} />
          ) : (
            // `shiki-fallback` is a CSS hook — the markdown.css rule sets
            // colour + whitespace-pre on this pre while we wait for Shiki.
            <pre className="shiki-body shiki-fallback m-0">{code}</pre>
          )}
          {lineCount > FOLD_LINE_THRESHOLD && !isSettling && (
            <button
              type="button"
              onClick={() => setExpanded(false)}
              title="Collapse code block"
              className={cn(
                FOLD_TOGGLE,
                "border-t border-[color-mix(in_srgb,var(--color-text)_7%,transparent)]",
              )}
            >
              <Icon name="minimize" size={12} />
              <span>Collapse</span>
            </button>
          )}
        </>
      )}
    </div>
  );
}

const FOLD_TOGGLE =
  "flex w-full items-center justify-center gap-1.5 border-0 bg-transparent px-4 py-2.5 font-sans text-[11.5px] font-semibold text-fg-muted tracking-normal transition-[background,color] duration-150 hover:bg-[color-mix(in_srgb,var(--color-text)_4%,transparent)] hover:text-fg";
