// Tool card — renders one agent tool invocation as a flat activity card
// (Geist / Codex aesthetic). A leading status-tinted icon chip + a stacked
// name / mono-detail column + trailing meta pills / running dot / chevron.
// Expands inline to the plugin-contributed preview (or ToolInspector
// fallback). Selected state drives the inspector pane via the workspace
// navigation wiring.
//
// Separation is by background delta (bg-surface on the canvas reading column),
// not a drop-shadow or grey border — see chat DESIGN notes. Error / requires-
// action tint the whole card so a failure reads at a glance even collapsed.
import type { IconName } from "@/ui";
import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import { Collapsible, Icon, StatusDot } from "@/ui";
import {
  toolIntent,
  toolMetaItems,
  type ToolMetaItem,
} from "@/plugins/builtin/agent/public/messagePresentation";
import { cn } from "@/lib/utils";
import {
  lookupToolActionOwner,
  lookupToolViewOpenerOwner,
  reportPluginError,
  TOOL_ACTION,
  TOOL_VIEW_OPENER,
  useExtensionPoint,
} from "@/plugins/sdk";
import { toolIconFor, toolRoutingKey } from "../public/toolIcon";
import { ToolPreview } from "./ToolPreview";

interface Props {
  tool: ToolCall;
  expanded: boolean;
  onToggleExpand: () => void;
}

export function ToolCard({ tool, expanded, onToggleExpand }: Props) {
  const running = tool.status === "running";
  const isError = tool.status === "err";
  const needsAction = tool.status === "requires-action";
  const actions = useExtensionPoint(TOOL_ACTION).filter((a) => !a.predicate || a.predicate(tool));
  const viewOpener = useExtensionPoint(TOOL_VIEW_OPENER).find((o) => o.predicate(tool));
  const onOpenView = viewOpener
    ? () => {
        void Promise.resolve(viewOpener.open(tool)).catch((err) => {
          const owner = lookupToolViewOpenerOwner(viewOpener.id) ?? "unknown";
          console.error(`[plugin] tool view opener ${viewOpener.id} threw:`, err);
          reportPluginError(owner, "command", err, `tool view opener: ${viewOpener.id}`);
        });
      }
    : undefined;
  const intent = toolIntent(tool);
  const metaItems = toolMetaItems(tool);

  // The error message takes over the detail line so a failure stays legible
  // even while collapsed; otherwise the arg summary (path / command / query).
  const detail = isError && tool.error ? tool.error : intent.detail;

  const actionClass = cn(
    "grid h-6 w-6 shrink-0 place-items-center rounded-md border-0 transition-[opacity,color,background-color]",
    isError
      ? "bg-canvas text-fg-muted opacity-100 shadow-[var(--shadow-control)] hover:text-fg"
      : "bg-transparent text-fg-faint opacity-0 group-hover:opacity-100 hover:bg-fg/[0.06] hover:text-fg",
  );

  return (
    <div data-slot="tool-card-root" className="group relative my-1">
      <div
        className={cn(
          "overflow-hidden rounded-[12px] transition-colors duration-150",
          isError ? "bg-negative/10" : needsAction ? "bg-warning/10" : "bg-surface",
        )}
      >
        <button
          data-slot="tool-card-trigger"
          type="button"
          aria-expanded={expanded}
          onClick={onToggleExpand}
          className={cn(
            "flex w-full items-center gap-3 px-3 py-2.5 text-left",
            "transition-colors duration-100",
            isError
              ? "hover:bg-negative/[0.06]"
              : needsAction
                ? "hover:bg-warning/[0.06]"
                : "hover:bg-fg/[0.03]",
            "focus-visible:outline-none focus-visible:shadow-[var(--shadow-focus)]",
          )}
        >
          {/* Leading status chip — tool glyph, tinted by status. */}
          <IconChip status={tool.status} tool={tool} />

          {/* Name + mono detail, stacked. */}
          <div className="flex min-w-0 flex-1 flex-col gap-0.5">
            <span
              title={intent.label}
              className={cn(
                "truncate text-[13px] font-medium leading-[1.3]",
                isError
                  ? "text-negative"
                  : needsAction
                    ? "text-warning"
                    : running
                      ? "text-accent"
                      : "text-fg",
              )}
            >
              {intent.label}
            </span>
            {detail && (
              <span
                title={typeof detail === "string" ? detail : undefined}
                className={cn(
                  "font-mono text-[12px] leading-[1.4]",
                  isError ? "break-words text-negative/80" : "truncate text-fg-muted",
                )}
              >
                {detail}
              </span>
            )}
          </div>

          {/* Trailing meta pills — inline status counts (+N / -N / matches …). */}
          <ToolMeta items={metaItems} running={running} />

          {/* Running indicator — accent pulse dot. */}
          {running && <StatusDot tone="running" className="ml-0.5" />}

          {/* Plugin actions — hover-reveal, or an always-visible retry chip on error. */}
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
              className={actionClass}
            >
              <Icon name={a.icon as IconName} size={12} />
            </button>
          ))}

          {/* Expand chevron — rotates -90° when collapsed. */}
          <Icon
            name="chevron-down"
            size={14}
            className={cn(
              "shrink-0 text-fg-faint transition-transform duration-150",
              !expanded && "-rotate-90",
            )}
          />
        </button>

        {/* Expanded inline preview — plugin renderer or ToolInspector fallback. */}
        <Collapsible open={expanded}>
          <div data-slot="tool-card-content" className="px-3 pb-3 pt-0.5">
            <ToolPreview tool={tool} onOpenView={onOpenView} />
          </div>
        </Collapsible>
      </div>
    </div>
  );
}

// Leading 28px chip. Keeps the per-tool glyph (so a glance tells read from
// write from search) while the fill/ink encodes status.
function IconChip({ status, tool }: { status: ToolCall["status"]; tool: ToolCall }) {
  const tone =
    status === "err"
      ? "bg-negative/15 text-negative"
      : status === "requires-action"
        ? "bg-warning/15 text-warning"
        : status === "ok"
          ? "bg-success/15 text-success"
          : status === "denied"
            ? "bg-fg/[0.06] text-fg-faint"
            : "bg-surface-2 text-fg-muted"; // running / pending
  const icon: IconName =
    status === "err"
      ? "x"
      : status === "requires-action"
        ? "alert"
        : status === "denied"
          ? "stop"
          : toolIconFor(toolRoutingKey(tool));
  return (
    <span
      data-slot="tool-card-status"
      className={cn("grid h-7 w-7 shrink-0 place-items-center rounded-[8px]", tone)}
    >
      <Icon name={icon} size={15} />
    </span>
  );
}

function ToolMeta({ items, running }: { items: ToolMetaItem[]; running: boolean }) {
  // The running state is carried by the dedicated pulse dot, so the "live"
  // word would be redundant next to it.
  const shown = running ? items.filter((item) => item.id !== "live") : items;
  if (shown.length === 0) return null;

  return (
    <div className="hidden shrink-0 items-center gap-1 sm:flex">
      {shown.map((item) => (
        <span
          key={item.id}
          className={cn(
            "inline-flex h-5 items-center rounded-pill px-2 font-mono text-[11px] font-medium",
            toolMetaToneClass(item.tone),
          )}
        >
          {item.label}
        </span>
      ))}
    </div>
  );
}

function toolMetaToneClass(tone: ToolMetaItem["tone"]): string {
  if (tone === "success") return "bg-success/15 text-success";
  if (tone === "negative") return "bg-negative/15 text-negative";
  return "bg-fg/[0.06] text-fg-muted";
}
