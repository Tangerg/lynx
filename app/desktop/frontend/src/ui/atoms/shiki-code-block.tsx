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
  /** Accessible name for the focusable preview scroll region. */
  previewLabel?: string;
}

interface HighlightedCode {
  lang: string;
  theme: string;
  code: string;
  html: string;
}

// We debounce `code` so the Shiki tokenizer (3-10ms per pass) doesn't
// run on every stream-reveal delta during streaming. While it's settling,
// raw code shows in a <pre> fallback.

export function ShikiCodeBlock({ lang, code, file, preview, previewLabel }: Props) {
  const t = useT();
  const shikiTheme = useShikiTheme();
  const isPreview = preview !== undefined;

  const [debouncedCode] = useDebouncedValue(code, { wait: 120 });
  const isSettling = code !== debouncedCode;

  // Seed from cache synchronously so a re-mount (scroll away/back, theme
  // toggle returning to a prior theme, MarkdownBlock memo invalidation
  // on a long history) skips both the async highlighter resolution and
  // the tokenizer call. Cache key is (lang, theme, exact-code).
  const cachedHtml = getCachedHighlight(lang, shikiTheme, debouncedCode);
  const [highlighted, setHighlighted] = useState<HighlightedCode | null>(() =>
    cachedHtml === undefined
      ? null
      : { lang, theme: shikiTheme, code: debouncedCode, html: cachedHtml },
  );
  const html =
    cachedHtml ??
    (highlighted?.lang === lang &&
    highlighted.theme === shikiTheme &&
    highlighted.code === debouncedCode
      ? highlighted.html
      : null);
  const wrapCode = useCodeWrapPreference();
  const { copied, copy } = useCopyFeedback(code);

  useEffect(() => {
    // Fast path — cache hit means we never wake the async highlighter.
    if (cachedHtml !== undefined) return;

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
          setHighlighted({ lang, theme: shikiTheme, code: debouncedCode, html: out });
        } catch {
          // Raw code remains the stable fallback for unsupported grammars.
        }
      })
      .catch(() => undefined);
    return () => {
      cancelled = true;
    };
  }, [cachedHtml, lang, debouncedCode, shikiTheme]);

  // Streaming → raw <pre> fallback; settled → swap to highlighted.
  // Falls back indefinitely if the highlighter never resolves.
  const showHighlighted = !isSettling && html !== null;

  return (
    // `shiki-block` is a CSS hook for markdown.css rules that style the
    // `<pre class="shiki">` + child `<code>` Shiki emits as a string.
    <div
      dir="ltr"
      data-variant={isPreview ? "preview" : "code"}
      data-markdown-copy="code-block"
      data-markdown-copy-text={code}
      className={cn(
        "shiki-block group/code my-3.5 overflow-hidden font-mono text-code",
        isPreview
          ? "group/code-snippet rounded-lg border-[0.5px] border-field bg-transparent"
          : "rounded-lg bg-sunken",
      )}
    >
      {/* Header — one material with the source body, like Codex. The quiet caption
          separates chrome from code without adding a second surface. Language then
          path stay left-aligned: they are one caption ("this TypeScript, from
          there"), and centring the path put the two halves of that sentence at
          opposite ends of a wide block. */}
      <div
        data-markdown-copy="exclude"
        className="flex items-center gap-2 bg-transparent px-2 py-1 font-sans text-ui-md"
      >
        <span
          className={cn(
            "shrink-0 text-fg-muted",
            isPreview
              ? "font-sans text-ui-md tracking-normal"
              : "font-sans text-ui-md font-normal tracking-normal normal-case",
          )}
        >
          {lang || "text"}
        </span>
        {file && (
          <span className="min-w-0 flex-1 truncate font-sans text-ui-md text-fg-muted">{file}</span>
        )}
        <span className="min-w-1 flex-1" />
        {!isPreview && (
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
          // Ordinary source keeps its primary action visible. A rendered preview
          // gives the artifact the visual lead and reveals copy on hover/focus.
          className={cn(
            copied ? "text-success" : "text-fg-faint hover:bg-hover hover:text-fg",
            isPreview &&
              "opacity-0 transition-opacity group-hover/code-snippet:opacity-100 group-focus-within/code-snippet:opacity-100",
          )}
        />
      </div>
      {isPreview ? (
        <div
          className="shiki-preview-body grid max-h-[calc(15lh+16px)] place-items-center overflow-auto p-2"
          role="region"
          aria-label={previewLabel}
          // Overflow regions need a keyboard entry point; the linter cannot
          // infer scrollability from the utility classes above.
          // oxlint-disable-next-line jsx-a11y/no-noninteractive-tabindex
          tabIndex={0}
        >
          {preview}
        </div>
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
