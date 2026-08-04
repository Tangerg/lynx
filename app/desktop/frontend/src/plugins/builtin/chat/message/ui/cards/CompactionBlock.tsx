// CompactionBlock — a context-compaction boundary (B10, 613). Renders as a
// slim, centered "⊟ Compacted N earlier messages" divider between turns; when
// the backend supplies a summary it expands inline on click.

import { useState } from "react";
import { Divider, Icon, TextButton } from "@/ui";
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
    <div>
      {/* The rules come from the Divider atom; this used to draw its own pair, at a
          third alpha for the same idea. Clickable when a summary is available. */}
      <Divider>
        {summary ? (
          <TextButton
            type="button"
            onClick={() => setOpen((v) => !v)}
            aria-expanded={open}
            className="inline-flex items-center gap-1 transition-colors hover:text-fg-muted"
          >
            <Icon name={open ? "chevron-up" : "chevron-down"} size="xs" />
            <span>{label}</span>
          </TextButton>
        ) : (
          <span className="inline-flex items-center gap-1">
            <Icon name="minimize" size="xs" />
            <span>{label}</span>
          </span>
        )}
      </Divider>
      {open && summary && (
        <div className="mx-auto mt-2 max-w-[640px] text-left text-ui-md leading-prose text-fg-muted">
          {summary}
        </div>
      )}
    </div>
  );
}
