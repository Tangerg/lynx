import { useEffect, useMemo, useState } from "react";
import { useThemeStore } from "@/state/themeStore";
import { Icon } from "@/components/common";
import { cn } from "@/lib/utils";
import { getHighlighter, resolveLang } from "@/lib/shiki";
import { useDebounce } from "use-debounce";
import { resolveScheme } from "@/plugins/sdk";

type Props = {
  lang: string;
  code: string;
  /**
   * Optional filename to display in the header. When set, the lang pill
   * sits on the left and the filename takes the centre column.
   */
  file?: string;
};

// ShikiCodeBlock — async syntax highlighting with theme-aware output.
//
// Why we debounce `code` instead of highlighting on every render:
// smooth-text reveals new chars at ~30 Hz during streaming, and each
// Shiki tokenization pass is 3-10ms. Hammering it every delta racks up
// hundreds of ms/sec on the main thread and freezes the chat. We hold
// onto the previous highlight while the stream is in flight and only
// re-tokenize once the code stops changing for 120ms — at that point
// the user is either between paragraphs or the block has closed. The
// LIVE code (pre-debounce) shows in a plain `<pre>` fallback in the
// meantime, which doubles as the loading state on first paint.
// Blocks longer than this default to collapsed (only while not streaming).
// Below the threshold we render in full so short snippets aren't hidden
// behind a click.
const FOLD_LINE_THRESHOLD = 24;

export function ShikiCodeBlock({ lang, code, file }: Props) {
  const themeId = useThemeStore((s) => s.theme);
  // Use the spec's scheme so custom themes (e.g. "solarized-dark") still
  // resolve to the correct shiki preset — comparing id === "light" would
  // miss third-party light themes.
  const scheme = resolveScheme(themeId);
  const shikiTheme = scheme === "light" ? "github-light" : "github-dark";

  const [debouncedCode] = useDebounce(code, 120);
  const isSettling = code !== debouncedCode;

  const [html, setHtml] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);
  const [expanded, setExpanded] = useState(false);

  const lineCount = useMemo(() => code.split("\n").length, [code]);
  // Auto-fold rules: long block + stream has settled + user hasn't expanded.
  // While the code is still streaming we leave it open — collapsing a
  // growing block hides the agent's progress.
  const folded = !expanded && !isSettling && lineCount > FOLD_LINE_THRESHOLD;

  useEffect(() => {
    let cancelled = false;
    getHighlighter()
      .then((h) => {
        if (cancelled) return;
        try {
          const resolvedLang = resolveLang(h, lang);
          const out = h.codeToHtml(debouncedCode, {
            lang: resolvedLang,
            theme: shikiTheme,
          });
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

  const onCopy = () => {
    try {
      navigator.clipboard?.writeText(code);
    } catch {
      /* ignore */
    }
    setCopied(true);
    setTimeout(() => setCopied(false), 1500);
  };

  // While streaming we show the live (un-highlighted) text — feels like a
  // terminal scrolling in. When the stream settles, we swap to the
  // syntax-highlighted version. If the highlighter never finished (e.g.
  // failed to load), we keep the fallback indefinitely.
  const showHighlighted = !isSettling && html !== null;

  return (
    // `shiki-block` class is kept as a DOM hook for the markdown.css
    // rules that style Shiki's emitted `.shiki` + `.shiki code` children
    // (those come from a string, not JSX, so Tailwind utilities can't
    // reach them). Everything else here is Tailwind utilities.
    <div
      className={cn(
        "shiki-block group/code my-3.5 overflow-hidden rounded-lg font-mono text-[12.5px]",
        "border border-[color-mix(in_srgb,var(--color-text)_10%,transparent)]",
        "bg-[color-mix(in_srgb,var(--color-text)_3%,transparent)]",
        folded && "folded",
      )}
    >
      <div className="grid grid-cols-[auto_1fr_auto] items-center gap-2.5 border-b border-[color-mix(in_srgb,var(--color-text)_7%,transparent)] pl-3.5 pr-3 py-2">
        <span className="font-sans text-[9.5px] font-semibold text-fg-faint tracking-normal normal-case">
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
          className="inline-flex items-center gap-1 rounded-md border-0 bg-transparent px-2 py-1 font-mono text-[10.5px] font-semibold text-fg-faint cursor-pointer transition-[opacity,color,background] duration-150 opacity-60 group-hover/code:opacity-100 hover:!text-fg hover:bg-[color-mix(in_srgb,var(--color-text)_8%,transparent)]"
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
              className={cn(FOLD_TOGGLE, "border-t border-[color-mix(in_srgb,var(--color-text)_7%,transparent)]")}
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
  "flex w-full items-center justify-center gap-1.5 border-0 bg-transparent px-4 py-2.5 font-sans text-[11.5px] font-semibold text-fg-muted tracking-normal cursor-pointer transition-[background,color] duration-150 hover:bg-[color-mix(in_srgb,var(--color-text)_4%,transparent)] hover:text-fg";
