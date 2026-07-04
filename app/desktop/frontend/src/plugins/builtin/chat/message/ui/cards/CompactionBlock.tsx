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
    <div className="my-3" data-slot="compaction-block">
      {/* Centered label flanked by faint hairlines (bg-fg delta, no cheap
          grey rule). Clickable when a summary is available to expand inline. */}
      <div className="flex items-center gap-3 text-[12px] text-fg-faint">
        <span className="h-px flex-1 bg-fg/[0.08]" />
        {summary ? (
          <button
            type="button"
            onClick={() => setOpen((v) => !v)}
            aria-expanded={open}
            className="inline-flex items-center gap-1 transition-colors hover:text-fg-muted"
          >
            <Icon name={open ? "chevron-up" : "chevron-down"} size={12} />
            <span>{label}</span>
          </button>
        ) : (
          <span className="inline-flex items-center gap-1">
            <Icon name="minimize" size={12} />
            <span>{label}</span>
          </span>
        )}
        <span className="h-px flex-1 bg-fg/[0.08]" />
      </div>
      {open && summary && (
        <div className="mx-auto mt-2 max-w-[640px] text-left text-[13px] leading-relaxed text-fg-muted">
          {summary}
        </div>
      )}
    </div>
  );
}
