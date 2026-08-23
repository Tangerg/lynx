import { useEffect, useState } from "react";
import type { BundledLanguage } from "shiki";

import { useLocalization } from "../../localization/Localization";

interface CodeBlockProps {
  code: string;
  language?: string;
}

type HighlightState =
  | { type: "plain" }
  | { type: "loading" }
  | { type: "highlighted"; html: string };

let shikiModule: Promise<typeof import("shiki")> | undefined;

export function CodeBlock({ code, language }: CodeBlockProps) {
  const { t } = useLocalization();
  const [wrap, setWrap] = useState(false);
  const [copyState, setCopyState] = useState<"idle" | "copied" | "failed">(
    "idle",
  );
  const [highlight, setHighlight] = useState<HighlightState>(
    language ? { type: "loading" } : { type: "plain" },
  );

  useEffect(() => {
    if (!language) {
      setHighlight({ type: "plain" });
      return;
    }
    let current = true;
    setHighlight({ type: "loading" });
    void highlightCode(code, language).then((html) => {
      if (current) {
        setHighlight(html ? { type: "highlighted", html } : { type: "plain" });
      }
    });
    return () => {
      current = false;
    };
  }, [code, language]);

  useEffect(() => {
    if (copyState === "idle") return;
    const timer = window.setTimeout(() => setCopyState("idle"), 1_600);
    return () => window.clearTimeout(timer);
  }, [copyState]);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(code);
      setCopyState("copied");
    } catch {
      setCopyState("failed");
    }
  };

  return (
    <section className="code-block" data-wrap={wrap}>
      <header>
        <span>{language || t("code.plainText")}</span>
        <div>
          <button
            type="button"
            aria-pressed={wrap}
            onClick={() => setWrap((current) => !current)}
          >
            {wrap ? t("code.noWrap") : t("code.wrap")}
          </button>
          <button type="button" onClick={() => void copy()}>
            {copyState === "copied"
              ? t("code.copied")
              : copyState === "failed"
                ? t("code.retryCopy")
                : t("code.copy")}
          </button>
        </div>
      </header>
      {highlight.type === "highlighted" ? (
        <div
          className="code-highlight"
          dangerouslySetInnerHTML={{ __html: highlight.html }}
        />
      ) : (
        <pre className="message-code" aria-busy={highlight.type === "loading"}>
          <code>{code}</code>
        </pre>
      )}
    </section>
  );
}

async function highlightCode(code: string, language: string) {
  try {
    shikiModule ??= import("shiki");
    const { bundledLanguages, codeToHtml } = await shikiModule;
    const languageID = normalizedLanguage(language);
    if (!isBundledLanguage(languageID, bundledLanguages)) return undefined;
    return await codeToHtml(code, {
      lang: languageID,
      theme: "github-dark",
    });
  } catch {
    return undefined;
  }
}

function normalizedLanguage(language: string) {
  const aliases: Record<string, string> = {
    golang: "go",
    js: "javascript",
    jsx: "jsx",
    py: "python",
    rb: "ruby",
    rs: "rust",
    shell: "bash",
    ts: "typescript",
    tsx: "tsx",
    yml: "yaml",
  };
  const normalized = language.trim().toLocaleLowerCase();
  return aliases[normalized] ?? normalized;
}

function isBundledLanguage(
  language: string,
  available: object,
): language is BundledLanguage {
  return language in available;
}
