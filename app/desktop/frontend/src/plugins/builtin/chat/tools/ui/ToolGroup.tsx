import { useState } from "react";
import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import { Collapsible, Icon } from "@/ui";
import { cn } from "@/lib/utils";
import { toolGroupModel, type ToolGroupPinnedState } from "../application/toolGroupModel";
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
  const [pinned, setPinned] = useState<ToolGroupPinnedState>(null);
  const model = toolGroupModel(tools, pinned);

  return (
    <div className="my-1">
      {/* Summary row — a flat header, separated by hover fill not a border. */}
      <button
        type="button"
        onClick={() => setPinned(model.nextPinned)}
        aria-expanded={model.expanded}
        className={cn(
          "flex w-full items-center gap-2 rounded-[10px] px-2.5 py-1.5 text-left",
          "transition-colors duration-100 hover:bg-fg/[0.03]",
          "focus-visible:outline-none focus-visible:shadow-[var(--shadow-focus)]",
        )}
      >
        <Icon name="search" size={14} className="shrink-0 text-fg-muted" />
        <span className="truncate text-[13px] font-medium text-fg-muted">{model.summary}</span>
        <span className="ml-auto shrink-0 rounded-pill bg-fg/[0.06] px-2 py-0.5 font-mono text-[11px] font-medium text-fg-muted">
          {model.count} calls
        </span>
        <Icon
          name="chevron-down"
          size={14}
          className={cn(
            "shrink-0 text-fg-faint transition-transform duration-150",
            !model.expanded && "-rotate-90",
          )}
        />
      </button>

      <Collapsible open={model.expanded}>
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
