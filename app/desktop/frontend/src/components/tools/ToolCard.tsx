import type { IconName } from "@/components/common";
import type { ToolCall } from "@/protocol/run/viewState";
import { AnimatePresence, motion } from "motion/react";
import { Icon } from "@/components/common";
import { swift } from "@/lib/motion";
import { cn } from "@/lib/utils";
import {
  lookupToolActionOwner,
  reportPluginError,
  TOOL_ACTION,
  useExtensionPoint,
} from "@/plugins/sdk";
import { toolIconFor, toolRoutingKey } from "./toolIcon";
import { ToolPreview } from "./ToolPreview";

interface Props {
  tool: ToolCall;
  selected: boolean;
  expanded: boolean;
  onToggleExpand: () => void;
  onOpenView: () => void;
}

// Status → text colour. Each state maps to a semantic token; the `run`
// state also gets a pulsing accent dot in the status pill. `denied` (HITL
// decline) is neutral — it's a user choice, not a failure.
const STATUS_TONE = {
  ok: "text-success",
  err: "text-negative",
  run: "text-accent",
  denied: "text-fg-muted",
} as const;
// Glyph instead of word — "Running / Done / Failed / Denied" → "● / ✓ / ✗ / ⊘"
// — gives the row an RPC-log voice (see DESIGN.md §8 "RPC log rule"). The
// pulsing dot for `run` is set up via the `before:` pseudo-element below.
const STATUS_GLYPH = { ok: "✓", err: "✗", run: "", denied: "⊘" } as const;

export function ToolCard({ tool, selected, expanded, onToggleExpand, onOpenView }: Props) {
  const status: keyof typeof STATUS_TONE = tool.status === "running" ? "run" : tool.status;
  const statusGlyph = STATUS_GLYPH[status];
  // Icon routes by the same key as the preview (kind for typed variants, tool
  // name for the generic envelope) — see toolRoutingKey.
  const toolIcon = toolIconFor(toolRoutingKey(tool));
  const actions = useExtensionPoint(TOOL_ACTION).filter((a) => !a.predicate || a.predicate(tool));
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
      {/* Header row contains nested <button> action affordances (run, view,
          expand) — turning the row itself into a button would emit invalid
          button-in-button HTML. Keep div + role + manual keyboard handling. */}
      <div
        // eslint-disable-next-line jsx-a11y/prefer-tag-over-role
        role="button"
        tabIndex={0}
        aria-expanded={expanded}
        className="grid grid-cols-[28px_minmax(0,1fr)_auto_auto_auto] items-center gap-2.5 px-3 py-1.5 focus-visible:outline-none focus-visible:shadow-[0_0_0_2px_var(--color-accent)]"
        onClick={onToggleExpand}
        onKeyDown={(e) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            onToggleExpand();
          }
        }}
      >
        <div
          className={cn(
            "grid h-5 w-5 shrink-0 place-items-center rounded-xs bg-transparent transition-colors",
            STATUS_TONE[status],
          )}
        >
          <Icon name={toolIcon} size={14} />
        </div>
        <div className="flex items-baseline gap-2 min-w-0">
          <span className="font-mono text-[12px] font-semibold text-fg tracking-[-0.005em]">
            {tool.fn}
          </span>
          {/* Args rendered as a parens-wrapped argument list, mono, so the
              full line reads as a function signature: `read({…})`. Tools whose
              key arg is baked into `fn` (command / fileEdit / search / webSearch
              / read, §4.4.2) carry no separate args — skip the parens entirely
              rather than print an empty `()`. */}
          {tool.args && (
            <span className="truncate font-mono text-[11.5px] text-fg-faint tracking-[-0.005em]">
              ({tool.args})
            </span>
          )}
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
                const owner = lookupToolActionOwner(a.id) ?? "unknown";

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
      {/* Tool-level failure reason (toolCall.error, §8.1 channel b) — shown
          inline so an "err" tool says *why*, not just goes red. */}
      {tool.status === "err" && tool.error && (
        <div className="px-3 pb-2 pl-[40px] font-mono text-[11px] leading-snug text-negative">
          {tool.error}
        </div>
      )}
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
    <div className="flex items-center gap-2.5 font-mono text-[10px] text-fg-faint tracking-normal normal-case">
      {tool.added != null && <span className="text-accent">+{tool.added}</span>}
      {tool.removed != null && <span className="text-negative">−{tool.removed}</span>}
      {tool.hits != null && <span>{tool.hits} matches</span>}
      {tool.exitCode != null && tool.exitCode !== 0 && (
        <span className="text-negative">exit {tool.exitCode}</span>
      )}
      <span>·</span>
      <span>{tool.duration}</span>
    </div>
  );
}
