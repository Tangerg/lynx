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
  onOpenView: () => void;
};

export function ToolCard({
  tool, selected, expanded, onToggleExpand, onOpenView,
}: Props) {
  const statusClass = tool.status === "running" ? "run" : tool.status === "ok" ? "ok" : "err";
  // Glyph instead of word — "Running / Done / Failed" → "● / ✓ / ✗" — gives the
  // row an RPC-log voice (see DESIGN.md §8 "RPC log rule"). The pulsing dot
  // for `running` is set up in CSS via `.tool-status.run::before` animation.
  const statusGlyph = tool.status === "running" ? "" : tool.status === "ok" ? "✓" : "✗";
  const actions = useToolActions().filter((a) => !a.predicate || a.predicate(tool));

  return (
    <div className={`tool-card ${selected ? "selected" : ""} ${expanded ? "expanded" : ""} ${tool.status === "running" ? "running" : ""}`}>
      <div className="tool-head" onClick={onToggleExpand}>
        <div className={`tool-icon ${statusClass}`}>
          <Icon name={toolIconFor(tool.fn)} size={14} />
        </div>
        <div className="tool-name">
          <span className="tool-fn">{tool.fn}</span>
          {/* Args rendered as parens-wrapped argument list, mono, so the
              full line reads as a function signature. Truncates with
              ellipsis if the args are long. */}
          <span className="tool-args">({tool.args})</span>
        </div>
        <ToolMeta tool={tool} />
        <div className={`tool-status ${statusClass}`} aria-label={tool.status}>
          <span aria-hidden="true">{statusGlyph}</span>
        </div>
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
            <ToolPreview tool={tool} onOpenView={onOpenView} />
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
