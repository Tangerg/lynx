import { useEffect, useState, type ReactNode } from "react";
import { useDebouncedValue } from "@tanstack/react-pacer";
import { useCopyFeedback } from "@/lib/useCopyFeedback";
import { measureShikiHighlight } from "@/lib/metrics";
import { getHighlighter, resolveLang } from "@/lib/highlight/shiki";
import { getCachedHighlight, setCachedHighlight } from "@/lib/highlight/shikiCache";
import { useShikiTheme } from "@/lib/highlight/useCodeHighlight";
import { cn } from "@/lib/classNames";
import { toggleCodeWrapPreference, useCodeWrapPreference } from "@/lib/codeWrapPreference";
import { useT } from "@/lib/i18n";
import { IconButton } from "./icon-button";

interface Props {
  lang: string;
  code: string;
  /**
   * Optional filename to display in the header. When set, the lang pill
   * sits on the left and the filename takes the centre column.
   */
  file?: string;
  /** Optional rendered interpretation of the source. The code remains the copy
   *  payload and the header remains shared; only the body changes. */
  preview?: ReactNode;
}

// We debounce `code` so the Shiki tokenizer (3-10ms per pass) doesn't
// run on every stream-reveal delta during streaming. While it's settling,
// raw code shows in a <pre> fallback.

export function ShikiCodeBlock({ lang, code, file, preview }: Props) {
  const t = useT();
  const shikiTheme = useShikiTheme();

  const [debouncedCode] = useDebouncedValue(code, { wait: 120 });
  const isSettling = code !== debouncedCode;

  // Seed from cache synchronously so a re-mount (scroll away/back, theme
  // toggle returning to a prior theme, MarkdownBlock memo invalidation
  // on a long history) skips both the async highlighter resolution and
  // the tokenizer call. Cache key is (lang, theme, exact-code).
  const [html, setHtml] = useState<string | null>(
    () => getCachedHighlight(lang, shikiTheme, debouncedCode) ?? null,
  );
  const wrapCode = useCodeWrapPreference();
  const { copied, copy } = useCopyFeedback(code);

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
        {!preview && (
          <IconButton
            icon={wrapCode ? "wrap-text" : "unfold-horizontal"}
            size="xs"
            active={wrapCode}
            aria-pressed={wrapCode}
            onClick={toggleCodeWrapPreference}
            title={t(wrapCode ? "message.code.wrap.disable" : "message.code.wrap.enable")}
            className="text-fg-faint hover:bg-hover hover:text-fg"
          />
        )}
        <IconButton
          icon={copied ? "check" : "copy"}
          size="xs"
          onClick={() => void copy()}
          title={copied ? t("message.code.copied") : t("message.code.copy")}
          // Visible at rest, not on hover. The bar's other content is a
          // three-letter language tag, and a block without a filename left it
          // holding one faint word — a 34px strip of nothing between the
          // paragraph and the code. Copying a block is also the thing anyone
          // does most with one, and the reference shows it standing.
          className={cn(copied ? "text-success" : "text-fg-faint hover:bg-hover hover:text-fg")}
        />
      </div>
      {preview ? (
        <div className="grid max-h-96 min-h-24 place-items-center overflow-auto p-2">{preview}</div>
      ) : showHighlighted ? (
        <div
          className="shiki-body"
          data-wrap={wrapCode}
          dangerouslySetInnerHTML={{ __html: html! }}
        />
      ) : (
        // `shiki-fallback` is a CSS hook — the markdown.css rule sets
        // colour + whitespace-pre on this pre while we wait for Shiki.
        <pre className="shiki-body shiki-fallback m-0" data-wrap={wrapCode}>
          {code}
        </pre>
      )}
    </div>
  );
}
