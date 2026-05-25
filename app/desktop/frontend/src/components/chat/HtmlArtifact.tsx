// HTML artifact card — when an agent emits a ```html code fence, we
// detect it and render the result two ways: a sandboxed iframe preview
// + the original source (Shiki-highlighted). Iframe is sandbox-only
// (`allow-scripts` but NOT `allow-same-origin`) so the embedded
// document can't reach our app's storage, cookies, parent frame, etc.

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

  // Auto-fall-through for tiny snippets — render as a normal code block.
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
                "rounded-sm px-2 py-0.5 text-[11px] font-sans font-medium cursor-pointer transition-colors",
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
          // sandbox without allow-same-origin → the embedded doc is a
          // separate, isolated origin. Scripts can run but can't read
          // our cookies / localStorage / parent window. No allow-forms
          // either (we don't want navigation away from the artifact).
          sandbox="allow-scripts"
          srcDoc={code}
          title="HTML artifact preview"
          className="block h-[420px] w-full border-0 bg-white"
        />
      ) : (
        // Reuse the ordinary Shiki block for the source view so the
        // user gets the same syntax-highlighting + copy affordance.
        <ShikiCodeBlock lang="html" code={code} />
      )}
    </div>
  );
}
