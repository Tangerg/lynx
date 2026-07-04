import { useEffect, useMemo, useRef, useState } from "react";
import { useDebounce } from "use-debounce";
import { Icon } from "@/ui/icons";
import { copyText } from "@/lib/clipboard";
import { measureShikiHighlight } from "@/lib/metrics";
import { getHighlighter, resolveLang } from "@/lib/markdown/shiki";
import { getCachedHighlight, setCachedHighlight } from "@/lib/markdown/shikiCache";
import { useShikiTheme } from "@/lib/markdown/useCodeHighlight";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";

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
  const t = useT();
  const shikiTheme = useShikiTheme();

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
    void copyText(code).then((ok) => {
      if (!ok) return; // clipboard unavailable — don't flash a false "Copied"
      setCopied(true);
      if (copyTimerRef.current !== null) window.clearTimeout(copyTimerRef.current);
      copyTimerRef.current = window.setTimeout(() => {
        setCopied(false);
        copyTimerRef.current = null;
      }, 1500);
    });
  };

  // Streaming → raw <pre> fallback; settled → swap to highlighted.
  // Falls back indefinitely if the highlighter never resolves.
  const showHighlighted = !isSettling && html !== null;

  return (
    // `shiki-block` is a CSS hook for markdown.css rules that style the
    // `<pre class="shiki">` + child `<code>` Shiki emits as a string.
    <div
      className={cn(
        "shiki-block group/code my-3 overflow-hidden rounded-md font-mono text-[13px]",
        "bg-surface-2",
        folded && "folded",
      )}
    >
      {/* Header — craft-aligned: flex row, lang left, copy right, subtle surface step. */}
      <div className="flex items-center justify-between gap-3 px-3 py-1.5">
        <span className="font-mono text-[10px] font-medium text-fg-faint uppercase tracking-wider">
          {lang || "text"}
        </span>
        {file && (
          <span className="truncate font-mono text-[11px] text-fg-muted flex-1 text-center">
            {file}
          </span>
        )}
        <button
          type="button"
          onClick={onCopy}
          title={copied ? t("message.code.copied") : t("message.code.copy")}
          className={cn(
            "grid h-6 w-6 place-items-center rounded border-0 bg-transparent transition-[opacity,color,background] duration-150",
            copied
              ? "text-success opacity-100"
              : "text-fg-faint opacity-0 group-hover/code:opacity-100 hover:text-fg hover:bg-fg/[0.05]",
          )}
        >
          <Icon name={copied ? "check" : "copy"} size={13} />
        </button>
      </div>
      {folded ? (
        <button
          type="button"
          onClick={() => setExpanded(true)}
          title={t("message.code.expand")}
          className={FOLD_TOGGLE}
        >
          <Icon name="code" size={12} />
          <span>{t("message.code.showLines", { count: lineCount })}</span>
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
              title={t("message.code.collapse")}
              className={FOLD_TOGGLE}
            >
              <Icon name="minimize" size={12} />
              <span>{t("message.code.collapseLabel")}</span>
            </button>
          )}
        </>
      )}
    </div>
  );
}

const FOLD_TOGGLE =
  "flex w-full items-center justify-center gap-1.5 border-0 bg-transparent px-4 py-2 font-sans text-[11.5px] font-medium text-fg-muted tracking-normal transition-[background,color] duration-150 hover:bg-fg/[0.02] hover:text-fg";
