// CompactionBlock — a context-compaction boundary (B10, docs/613). Renders as a
// slim, centered "⊟ Compacted N earlier messages" divider between turns; when
// the backend supplies a summary it expands inline on click.

import { useState } from "react";
import { Icon } from "@/components/common";

export function CompactionBlock({
  summary,
  droppedMessages,
}: {
  summary?: string;
  droppedMessages?: number;
}) {
  const [open, setOpen] = useState(false);
  const label =
    droppedMessages && droppedMessages > 0
      ? `Compacted ${droppedMessages} earlier message${droppedMessages === 1 ? "" : "s"}`
      : "Context compacted";

  const pill =
    "flex items-center gap-1.5 rounded-full bg-surface-2 px-2.5 py-1 font-mono text-[11px] tracking-tight text-fg-faint light:bg-surface-3";

  if (!summary) {
    return (
      <div className="my-1 flex justify-center py-1">
        <span className={pill}>
          <Icon name="minimize" size={12} />
          {label}
        </span>
      </div>
    );
  }

  return (
    <div className="my-1 flex flex-col items-center gap-1.5 py-1">
      <button
        type="button"
        className={`${pill} hover:text-fg-muted`}
        onClick={() => setOpen((v) => !v)}
        aria-expanded={open}
      >
        <Icon name="minimize" size={12} />
        <span>{label}</span>
        <Icon name={open ? "chevron-up" : "chevron-down"} size={12} />
      </button>
      {open && (
        <div className="max-w-[640px] rounded-lg bg-surface-2 px-3 py-2 text-[13px] leading-relaxed text-fg-muted light:bg-surface-3">
          {summary}
        </div>
      )}
    </div>
  );
}
