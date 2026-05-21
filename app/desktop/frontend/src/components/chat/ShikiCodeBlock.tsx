import { useEffect, useMemo, useState } from "react";
import { useUIStore } from "@/state/uiStore";
import { Icon } from "@/components/common";
import { getHighlighter, resolveLang } from "@/lib/shiki";
import { useDebouncedValue } from "@/lib/useDebouncedValue";

type Props = {
  lang: string;
  code: string;
  /**
   * Optional filename to display in the header. When set, the lang pill
   * sits on the left and the filename takes the centre column — same
   * shape as the legacy CodeBlock so the code-proposal plugin keeps
   * working unchanged.
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
export function ShikiCodeBlock({ lang, code, file }: Props) {
  const theme = useUIStore((s) => s.theme);
  const shikiTheme = theme === "light" ? "github-light" : "github-dark";

  const debouncedCode = useDebouncedValue(code, 120);
  const isSettling = code !== debouncedCode;

  const [html, setHtml] = useState<string | null>(null);
  const [copied, setCopied] = useState(false);

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

  const displayLang = useMemo(() => lang || "text", [lang]);

  // While streaming we show the live (un-highlighted) text — feels like a
  // terminal scrolling in. When the stream settles, we swap to the
  // syntax-highlighted version. If the highlighter never finished (e.g.
  // failed to load), we keep the fallback indefinitely.
  const showHighlighted = !isSettling && html !== null;

  return (
    <div className="shiki-block">
      <div className="shiki-block-head">
        <span className="lang">{displayLang}</span>
        {file ? <span className="fname">{file}</span> : <span aria-hidden="true" />}
        <button className="copy" type="button" onClick={onCopy}>
          <Icon name={copied ? "check" : "file"} size={11} />
          {copied ? "Copied" : "Copy"}
        </button>
      </div>
      {showHighlighted ? (
        <div className="shiki-body" dangerouslySetInnerHTML={{ __html: html! }} />
      ) : (
        <pre className="shiki-body shiki-fallback">{code}</pre>
      )}
    </div>
  );
}
