import { useEffect, useMemo, useRef, useState } from "react";
import { useDebounce } from "use-debounce";
import { Icon } from "@/ui/icons";
import { copyText } from "@/lib/clipboard";
import { measureShikiHighlight } from "@/lib/metrics";
import { getHighlighter, resolveLang } from "@/lib/highlight/shiki";
import { getCachedHighlight, setCachedHighlight } from "@/lib/highlight/shikiCache";
import { useShikiTheme } from "@/lib/highlight/useCodeHighlight";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { IconButton } from "./icon-button";
import { Pressable } from "./pressable";

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
        "shiki-block group/code my-3 overflow-hidden rounded-md font-mono text-code",
        "bg-sunken",
        folded && "folded",
      )}
    >
      {/* Header — the card's own material over the recessed body, so the bar
          reads as the block's lid rather than as the first line of code. Language
          then path, both left-aligned: they are one caption ("this TypeScript,
          from there"), and centring the path put the two halves of that sentence
          at opposite ends of a wide block. */}
      <div className="flex items-center gap-2.5 bg-card px-3 py-1.5">
        <span className="shrink-0 font-mono text-ui-2xs font-medium uppercase tracking-wider text-fg-faint">
          {lang || "text"}
        </span>
        {file && (
          <span className="min-w-0 flex-1 truncate font-mono text-ui-xs text-fg-muted">{file}</span>
        )}
        <span className="min-w-1 flex-1" />
        <IconButton
          icon={copied ? "check" : "copy"}
          size="xs"
          onClick={onCopy}
          title={copied ? t("message.code.copied") : t("message.code.copy")}
          // Visible at rest, not on hover. The bar's other content is a
          // three-letter language tag, and a block without a filename left it
          // holding one faint word — a 34px strip of nothing between the
          // paragraph and the code. Copying a block is also the thing anyone
          // does most with one, and the reference shows it standing.
          className={cn(copied ? "text-success" : "text-fg-faint hover:bg-hover hover:text-fg")}
        />
      </div>
      {folded ? (
        <Pressable
          type="button"
          onClick={() => setExpanded(true)}
          title={t("message.code.expand")}
          className={FOLD_TOGGLE}
        >
          <Icon name="code" size="xs" />
          <span>{t("message.code.showLines", { count: lineCount })}</span>
        </Pressable>
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
            <Pressable
              type="button"
              onClick={() => setExpanded(false)}
              title={t("message.code.collapse")}
              className={FOLD_TOGGLE}
            >
              <Icon name="minimize" size="xs" />
              <span>{t("message.code.collapseLabel")}</span>
            </Pressable>
          )}
        </>
      )}
    </div>
  );
}

const FOLD_TOGGLE =
  "flex w-full items-center justify-center gap-1.5 border-0 bg-transparent px-4 py-2 font-sans text-ui-sm font-medium text-fg-muted tracking-normal transition-[background,color] duration-[var(--dur-fast)] hover:bg-hover hover:text-fg";
