import { AnimatePresence, motion } from "motion/react";
import { Icon, type IconName } from "@/components/common";
import { swift } from "@/lib/motion";
import { reportPluginError, usePluginStore, useToolActions } from "@/plugins/sdk";
import type { ToolCall } from "@/protocol/agui/viewState";
import { toolIconFor } from "./toolIcon";
import { ToolPreview } from "./ToolPreview";

type Props = {
  tool: ToolCall;
  selected: boolean;
  expanded: boolean;
  onToggleExpand: () => void;
  onOpenInspector: () => void;
};

export function ToolCard({
  tool, selected, expanded, onToggleExpand, onOpenInspector,
}: Props) {
  const statusClass = tool.status === "running" ? "run" : tool.status === "ok" ? "ok" : "err";
  const statusLabel = tool.status === "running" ? "Running" : tool.status === "ok" ? "Done" : "Failed";
  const actions = useToolActions().filter((a) => !a.predicate || a.predicate(tool));

  return (
    <div className={`tool-card ${selected ? "selected" : ""} ${expanded ? "expanded" : ""}`}>
      <div className="tool-head" onClick={onToggleExpand}>
        <div className={`tool-icon ${statusClass}`}>
          <Icon name={toolIconFor(tool.fn)} size={14} />
        </div>
        <div className="tool-name">
          <span className="tool-fn">{tool.fn}</span>
          <span className="tool-args">{tool.args}</span>
        </div>
        <ToolMeta tool={tool} />
        <div className={`tool-status ${statusClass}`}>{statusLabel}</div>
        {actions.map((a) => (
          <button
            key={a.id}
            className="tool-action"
            title={a.title}
            onClick={(e) => {
              e.stopPropagation();
              void Promise.resolve(a.run(tool)).catch((err) => {
                const owner = usePluginStore.getState().toolActions.get(a.id)?.pluginName ?? "unknown";
                // eslint-disable-next-line no-console
                console.error(`[plugin] tool action ${a.id} threw:`, err);
                reportPluginError(owner, "command", err, `tool action: ${a.id}`);
              });
            }}
          >
            <Icon name={a.icon as IconName} size={12} />
          </button>
        ))}
        <button
          className="tool-expand"
          title={expanded ? "Collapse" : "Expand preview"}
          onClick={(e) => { e.stopPropagation(); onToggleExpand(); }}
        >
          <Icon name={expanded ? "minimize" : "more"} size={12} />
        </button>
      </div>
      <AnimatePresence initial={false}>
        {expanded && (
          <motion.div
            key="preview"
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={swift}
            style={{ overflow: "hidden" }}
          >
            <ToolPreview tool={tool} onOpenInspector={onOpenInspector} />
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}

function ToolMeta({ tool }: { tool: ToolCall }) {
  return (
    <div className="tool-meta">
      {tool.added != null   && <span style={{ color: "var(--color-accent)" }}>+{tool.added}</span>}
      {tool.removed != null && <span style={{ color: "var(--color-negative)" }}>−{tool.removed}</span>}
      {tool.hits != null    && <span>{tool.hits} matches</span>}
      {tool.lines != null && tool.lines > 0 && tool.added == null && tool.hits == null && (
        <span>{tool.lines} lines</span>
      )}
      <span>·</span>
      <span>{tool.duration}</span>
    </div>
  );
}
