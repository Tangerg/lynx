import { useState } from "react";
import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import { toolIconFor } from "@/plugins/builtin/chat/tools/public/toolIcon";
import { AgentActivityDisclosure } from "@/ui/agent";
import { useT } from "@/lib/i18n";
import { toolGroupModel, type ToolGroupPinnedState } from "../application/toolGroupModel";
import { ToolGroupMember } from "./ToolGroupMember";

interface Props {
  tools: ToolCall[];
  onSelectTool: (id: string) => void;
  expandedIds: Set<string>;
  onToggleExpand: (id: string) => void;
  /** The turn has moved on to answering; see messageBlockRenderUnits. */
  superseded?: boolean;
}

/**
 * A run of adjacent read-only tool calls, folded into one collapsible summary
 * row so a long agent turn stays scannable. Auto-expands while any child is
 * still running or has errored, then settles closed once they finish — unless
 * the user has pinned it open or closed.
 *
 * The `line` shell is a summary row with indented children and no enclosing card.
 * Its children are read-only calls, so they
 * are lines too — a card around a stack of lines would put the weight back.
 */
export function ToolGroup({ tools, onSelectTool, expandedIds, onToggleExpand, superseded }: Props) {
  const [pinned, setPinned] = useState<ToolGroupPinnedState>(null);
  const t = useT();
  const model = toolGroupModel(t, tools, pinned, superseded);

  return (
    <AgentActivityDisclosure
      icon={toolIconFor(model.dominantTool)}
      shell="line"
      label={model.summary}
      trailing={
        <span className="font-mono text-ui-xs font-medium tabular-nums text-fg-muted">
          {t("tools.group.calls", { count: model.count })}
        </span>
      }
      open={model.expanded}
      onToggle={() => setPinned(model.nextPinned)}
      stickyHeader
    >
      {/* The expanded group remains one work narrative. Each member keeps its own
          mark and line rhythm; divider rules would turn it back into a table. */}
      <div className="flex flex-col gap-1">
        {tools.map((tool) => (
          <ToolGroupMember
            key={tool.id}
            tool={tool}
            expanded={expandedIds.has(tool.id)}
            onToggleExpand={() => {
              onSelectTool(tool.id);
              onToggleExpand(tool.id);
            }}
          />
        ))}
      </div>
    </AgentActivityDisclosure>
  );
}
