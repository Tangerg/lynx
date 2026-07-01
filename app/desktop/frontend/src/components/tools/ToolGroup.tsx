import { useState } from "react";
import type { ToolCall } from "@/protocol/run/viewState";
import { Collapsible, Icon } from "@/components/common";
import {
  summarizeToolGroup,
  toolGroupNeedsAttention,
} from "@/plugins/builtin/agent/public/messagePresentation";
import { cn } from "@/lib/utils";
import { ToolCard } from "./ToolCard";

interface Props {
  tools: ToolCall[];
  onSelectTool: (id: string) => void;
  expandedIds: Set<string>;
  onToggleExpand: (id: string) => void;
}

/**
 * A run of adjacent read-only tool calls, folded into one collapsible summary
 * row so a long agent turn stays scannable. Auto-expands while any child is
 * still running or has errored, then settles closed once they finish — unless
 * the user has pinned it open or closed. The group is a quiet vertical stack:
 * a summary row + indented child activity rows, no enclosing card.
 */
export function ToolGroup({ tools, onSelectTool, expandedIds, onToggleExpand }: Props) {
  const needsAttention = toolGroupNeedsAttention(tools);
  // null = follow `needsAttention` (open while live/errored, closed when done);
  // a boolean = the user has pinned it that way.
  const [pinned, setPinned] = useState<boolean | null>(null);
  const expanded = pinned ?? needsAttention;

  return (
    <div className="my-0.5">
      {/* Summary row — bare text line, no bg, no border, no surface. */}
      <button
        type="button"
        onClick={() => setPinned(!expanded)}
        aria-expanded={expanded}
        className={cn(
          "flex w-full items-center gap-2 px-2 py-0.5 text-left",
          "focus-visible:outline-none focus-visible:shadow-[0_0_0_2px_var(--color-accent)]",
        )}
      >
        <Icon
          name="chevron-down"
          size={12}
          className={cn(
            "shrink-0 text-fg-faint transition-transform duration-150",
            !expanded && "-rotate-90",
          )}
        />
        <Icon name="search" size={13} className="shrink-0 text-fg-muted" />
        <span className="truncate text-[13px] font-medium text-fg-muted">
          {summarizeToolGroup(tools)}
        </span>
        <span className="ml-auto shrink-0 font-mono text-[11px] text-fg-faint">
          {tools.length} calls
        </span>
      </button>

      <Collapsible open={expanded}>
        <div className="pl-4">
          {tools.map((t) => (
            <ToolCard
              key={t.id}
              tool={t}
              expanded={expandedIds.has(t.id)}
              onToggleExpand={() => {
                onSelectTool(t.id);
                onToggleExpand(t.id);
              }}
            />
          ))}
        </div>
      </Collapsible>
    </div>
  );
}
