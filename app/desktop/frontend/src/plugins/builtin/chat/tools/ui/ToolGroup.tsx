import { useState } from "react";
import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import { AgentActivityDisclosure } from "@/ui/agent";
import { useT } from "@/lib/i18n";
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
  const t = useT();
  const model = toolGroupModel(t, tools, pinned);

  return (
    <AgentActivityDisclosure
      icon="search"
      label={model.summary}
      trailing={
        <span className="font-mono text-ui-xs font-medium tabular-nums text-fg-muted">
          {t("tools.group.calls", { count: model.count })}
        </span>
      }
      open={model.expanded}
      onToggle={() => setPinned(model.nextPinned)}
    >
      {tools.map((tool) => (
        <ToolCard
          key={tool.id}
          tool={tool}
          expanded={expandedIds.has(tool.id)}
          onToggleExpand={() => {
            onSelectTool(tool.id);
            onToggleExpand(tool.id);
          }}
        />
      ))}
    </AgentActivityDisclosure>
  );
}
