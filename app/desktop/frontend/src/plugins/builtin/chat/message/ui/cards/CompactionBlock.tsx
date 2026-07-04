// CompactionBlock — a context-compaction boundary (B10, 613). Renders as a
// slim, centered "⊟ Compacted N earlier messages" divider between turns; when
// the backend supplies a summary it expands inline on click.

import { useState } from "react";
import { Icon } from "@/ui";
import { useT } from "@/lib/i18n";

export function CompactionBlock({
  summary,
  droppedMessages,
}: {
  summary?: string;
  droppedMessages?: number;
}) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const label =
    droppedMessages && droppedMessages > 0
      ? t("compaction.compactedN", { count: droppedMessages })
      : t("compaction.compacted");

  return (
    <div className="my-3 text-center" data-slot="compaction-block">
      {summary ? (
        <button
          type="button"
          onClick={() => setOpen((v) => !v)}
          aria-expanded={open}
          className="relative inline-flex items-center gap-1 text-[11px] text-fg-faint bg-bg px-2 -mt-[9px] hover:text-fg-muted transition-colors"
        >
          <Icon name={open ? "chevron-up" : "chevron-down"} size={12} />
          <span>{label}</span>
        </button>
      ) : (
        <span className="relative inline-flex items-center gap-1 text-[11px] text-fg-faint bg-bg px-2 -mt-[9px]">
          <Icon name="minimize" size={12} />
          <span>{label}</span>
        </span>
      )}
      {open && summary && (
        <div className="max-w-[640px] mx-auto mt-2 text-[13px] leading-relaxed text-fg-muted text-left">
          {summary}
        </div>
      )}
    </div>
  );
}
