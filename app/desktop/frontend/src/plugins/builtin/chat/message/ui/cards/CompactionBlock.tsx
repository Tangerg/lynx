// CompactionBlock — a context-compaction boundary (B10, 613). Renders as a
// quiet Codex activity row between turns; when the backend supplies a summary
// it expands inline on click without turning the context boundary into a card.

import { useId, useState } from "react";
import { Collapsible, Icon, TextButton } from "@/ui";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/classNames";

export function CompactionBlock({ summary }: { summary?: string; droppedMessages?: number }) {
  const t = useT();
  const [open, setOpen] = useState(false);
  const panelId = useId();
  const label = t("compaction.compacted");

  return (
    <div data-slot="agent-activity-item" className="flex min-w-0 flex-col">
      {summary ? (
        <TextButton
          type="button"
          size="sm"
          onClick={() => setOpen((value) => !value)}
          aria-label={label}
          aria-expanded={open}
          aria-controls={panelId}
          className="group/compaction max-w-full self-start py-1.5"
        >
          <Icon name="minimize" size="xs" className="shrink-0 text-fg-faint" />
          <span className="min-w-0 truncate">{label}</span>
          <Icon
            name="chevron-down"
            size="xs"
            className={cn(
              "shrink-0 text-fg-faint opacity-0 transition-[opacity,transform] duration-[var(--dur-fast)] group-hover/compaction:opacity-100 group-focus-visible/compaction:opacity-100",
              open && "rotate-180 opacity-100",
            )}
          />
        </TextButton>
      ) : (
        <div className="inline-flex min-w-0 self-start items-center gap-1.5 py-1.5 text-ui-sm text-fg-muted">
          <Icon name="minimize" size="xs" className="shrink-0 text-fg-faint" />
          <span className="min-w-0 truncate">{label}</span>
        </div>
      )}
      {summary && (
        <Collapsible open={open}>
          <div
            id={panelId}
            className="mt-1.5 ml-5 max-w-[640px] whitespace-pre-wrap text-left text-ui-sm leading-prose text-fg-muted"
          >
            {summary}
          </div>
        </Collapsible>
      )}
    </div>
  );
}
