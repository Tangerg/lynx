import { AnimatePresence, motion } from "motion/react";
import { Icon, type IconName } from "@/components/common";
import { cn } from "@/lib/utils";
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

// Status → text colour. Three states map to three semantic tokens; the
// `run` state also gets a pulsing accent dot in the status pill.
const STATUS_TONE = {
  ok: "text-success",
  err: "text-negative",
  run: "text-accent",
} as const;

export function ToolCard({ tool, selected, expanded, onToggleExpand, onOpenView }: Props) {
  const status: keyof typeof STATUS_TONE =
    tool.status === "running" ? "run" : tool.status === "ok" ? "ok" : "err";
  // Glyph instead of word — "Running / Done / Failed" → "● / ✓ / ✗" — gives the
  // row an RPC-log voice (see DESIGN.md §8 "RPC log rule"). The pulsing dot
  // for `running` is set up via the `before:` pseudo-element below.
  const statusGlyph = tool.status === "running" ? "" : tool.status === "ok" ? "✓" : "✗";
  const actions = useToolActions().filter((a) => !a.predicate || a.predicate(tool));
  const running = tool.status === "running";

  return (
    <div
      className={cn(
        // `tool-card` (raw class) is kept as a hook for the
        // `.tool-card.running::before` rotating conic-gradient border
        // animation defined in tool.css — it uses @property + mask, not
        // expressible cleanly in Tailwind. Everything else here is utilities.
        "tool-card group relative my-1.5 overflow-hidden rounded-md border border-transparent cursor-pointer transition-[background,border-color,transform] duration-150",
        !selected && "hover:bg-line hover:border-line-soft hover:-translate-y-px",
        selected && "bg-line border-line-soft",
        running && "running",
      )}
    >
      <div
        className="grid grid-cols-[28px_minmax(0,1fr)_auto_auto_auto] items-center gap-2.5 px-3 py-1.5"
        onClick={onToggleExpand}
      >
        <div
          className={cn(
            "grid h-5 w-5 shrink-0 place-items-center rounded-xs bg-transparent transition-colors",
            status === "ok"
              ? "text-success"
              : status === "err"
                ? "text-negative"
                : status === "run"
                  ? "text-accent"
                  : "text-fg-faint group-hover:text-fg-muted",
          )}
        >
          <Icon name={toolIconFor(tool.fn)} size={14} />
        </div>
        <div className="flex items-baseline gap-2 min-w-0">
          <span className="font-mono text-[12px] font-semibold text-fg tracking-[-0.005em]">
            {tool.fn}
          </span>
          {/* Args rendered as parens-wrapped argument list, mono, so the
              full line reads as a function signature. Truncates with
              ellipsis if the args are long. */}
          <span className="truncate font-mono text-[11.5px] text-fg-faint tracking-[-0.005em]">
            ({tool.args})
          </span>
        </div>
        <ToolMeta tool={tool} />
        <div
          aria-label={tool.status}
          className={cn(
            "rounded-sm px-1.5 py-0.5 font-mono text-[10px] font-semibold tracking-normal normal-case",
            STATUS_TONE[status],
            status === "run" &&
              "inline-flex items-center gap-1.5 before:content-[''] before:h-1.5 before:w-1.5 before:rounded-full before:bg-accent before:shadow-[0_0_6px_var(--color-accent)] before:animate-pulse-dot",
          )}
        >
          <span aria-hidden="true">{statusGlyph}</span>
        </div>
        {actions.map((a) => (
          <button
            key={a.id}
            type="button"
            title={a.title}
            onClick={(e) => {
              e.stopPropagation();
              void Promise.resolve(a.run(tool)).catch((err) => {
                const owner =
                  usePluginStore.getState().toolActions.get(a.id)?.pluginName ?? "unknown";
                // eslint-disable-next-line no-console
                console.error(`[plugin] tool action ${a.id} threw:`, err);
                reportPluginError(owner, "command", err, `tool action: ${a.id}`);
              });
            }}
            className={ACTION_BTN}
          >
            <Icon name={a.icon as IconName} size={12} />
          </button>
        ))}
        <button
          type="button"
          title={expanded ? "Collapse" : "Expand preview"}
          onClick={(e) => {
            e.stopPropagation();
            onToggleExpand();
          }}
          className={ACTION_BTN}
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

// Shared button style for inline action + expand glyphs.
const ACTION_BTN =
  "grid h-5.5 w-5.5 place-items-center rounded-xs border-0 bg-transparent text-fg-faint cursor-pointer transition-colors hover:bg-surface-3 hover:text-fg";

function ToolMeta({ tool }: { tool: ToolCall }) {
  return (
    <div className="flex items-center gap-2.5 font-mono text-[10px] text-fg-faint tabular-nums tracking-normal normal-case">
      {tool.added != null && <span className="text-accent">+{tool.added}</span>}
      {tool.removed != null && <span className="text-negative">−{tool.removed}</span>}
      {tool.hits != null && <span>{tool.hits} matches</span>}
      {tool.lines != null && tool.lines > 0 && tool.added == null && tool.hits == null && (
        <span>{tool.lines} lines</span>
      )}
      <span>·</span>
      <span>{tool.duration}</span>
    </div>
  );
}
