// HTML artifact card: preview + source tabs for a ```html fence.
// Iframe uses `allow-scripts` without `allow-same-origin` so the
// embedded doc lands in an opaque origin and can't read our cookies,
// storage, or reach into the parent frame.

import { useState } from "react";
import { Icon } from "@/components/common";
import { cn } from "@/lib/utils";
import { ShikiCodeBlock } from "./ShikiCodeBlock";

interface Props {
  code: string;
}

type Tab = "preview" | "source";

// Threshold for treating a code block as a "full document" worthy of
// its own preview frame. Tiny snippets (<= a single tag) just render
// as regular code so we don't put a 200px iframe around `<br>`.
const MIN_PREVIEW_LEN = 60;

function looksLikeDoc(code: string): boolean {
  if (code.length < MIN_PREVIEW_LEN) return false;
  // Either explicitly a document, or anything with at least two distinct
  // tags. Avoids false positives on a single `<div>` snippet.
  if (/<!doctype/i.test(code) || /<html[\s>]/i.test(code)) return true;
  const tags = code.match(/<\w+/g) ?? [];
  return tags.length >= 2;
}

export function HtmlArtifact({ code }: Props) {
  const [tab, setTab] = useState<Tab>("preview");

  if (!looksLikeDoc(code)) {
    return <ShikiCodeBlock lang="html" code={code} />;
  }

  return (
    <div className="my-3.5 overflow-hidden rounded-lg border border-line bg-surface">
      <div className="flex items-center justify-between border-b border-line px-3 py-1.5">
        <div className="inline-flex items-center gap-2">
          <Icon name="globe" size={12} className="text-fg-faint" />
          <span className="font-mono text-[11px] font-semibold text-fg-faint">HTML artifact</span>
        </div>
        <div className="inline-flex items-center gap-1 rounded bg-surface-2 p-0.5">
          {(["preview", "source"] as const).map((t) => (
            <button
              key={t}
              type="button"
              onClick={() => setTab(t)}
              className={cn(
                "rounded-sm px-2 py-0.5 text-[11px] font-sans font-medium transition-colors",
                tab === t
                  ? "bg-surface text-fg shadow-[inset_0_1px_0_rgba(255,255,255,0.03)]"
                  : "bg-transparent text-fg-muted hover:text-fg",
              )}
            >
              {t === "preview" ? "Preview" : "Source"}
            </button>
          ))}
        </div>
      </div>
      {tab === "preview" ? (
        <iframe
          // `allow-scripts` without `allow-same-origin`: the doc runs
          // but is treated as a foreign origin. No `allow-forms` —
          // we don't want navigation to leave the artifact.
          sandbox="allow-scripts"
          srcDoc={code}
          title="HTML artifact preview"
          className="block h-[420px] w-full border-0 bg-white"
        />
      ) : (
        <ShikiCodeBlock lang="html" code={code} />
      )}
    </div>
  );
}
