// HTML artifact card: preview + source tabs for a ```html fence.
// Iframe uses `allow-scripts` without `allow-same-origin` so the
// embedded doc lands in an opaque origin and can't read our cookies,
// storage, or reach into the parent frame.

import { useState } from "react";
import { Icon, Segmented, ShikiCodeBlock } from "@/ui";
import { useT } from "@/lib/i18n";

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
  const t = useT();
  const [tab, setTab] = useState<Tab>("preview");

  if (!looksLikeDoc(code)) {
    return <ShikiCodeBlock lang="html" code={code} />;
  }

  return (
    <div className="my-3.5 overflow-hidden rounded-lg bg-surface">
      <div className="flex items-center justify-between px-3 py-1.5">
        <div className="inline-flex items-center gap-2">
          <Icon name="globe" size={12} className="text-fg-faint" />
          <span className="font-mono text-ui-sm font-semibold text-fg-faint">
            {t("markdown.htmlArtifact")}
          </span>
        </div>
        {/* The library control, not a second one built to look like it. This was a
            hand-rolled well-and-chip pair whose lift came from a literal white
            inset — a value picked for the dark theme, which meant the light theme
            got a selected tab with no lift at all. */}
        <Segmented
          ariaLabel={t("message.html.tabsAria")}
          value={tab}
          onChange={setTab}
          options={[
            { value: "preview", label: t("message.html.tab.preview") },
            { value: "source", label: t("message.html.tab.source") },
          ]}
        />
      </div>
      {tab === "preview" ? (
        <iframe
          // `allow-scripts` without `allow-same-origin`: the doc runs
          // but is treated as a foreign origin. No `allow-forms` —
          // we don't want navigation to leave the artifact.
          sandbox="allow-scripts"
          srcDoc={code}
          title={t("message.html.preview")}
          className="block h-[420px] w-full border-0 bg-white"
        />
      ) : (
        <ShikiCodeBlock lang="html" code={code} />
      )}
    </div>
  );
}
