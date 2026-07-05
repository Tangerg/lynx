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
import { type ToolMetaItem } from "@/plugins/builtin/agent/public/messagePresentation";
import { cn } from "@/lib/utils";
import {
  lookupToolActionOwner,
  lookupToolViewOpenerOwner,
  reportPluginError,
  TOOL_ACTION,
  TOOL_VIEW_OPENER,
  useExtensionPoint,
} from "@/plugins/sdk";
import {
  toolCardActions,
  toolCardModel,
  toolCardViewOpener,
  visibleToolMetaItems,
} from "../application/toolCardModel";
import { toolIconFor, toolRoutingKey } from "../public/toolIcon";
import { ToolPreview } from "./ToolPreview";

interface Props {
  tool: ToolCall;
  expanded: boolean;
  onToggleExpand: () => void;
}

export function ToolCard({ tool, expanded, onToggleExpand }: Props) {
  const model = toolCardModel(tool);
  const allActions = useExtensionPoint(TOOL_ACTION);
  const allViewOpeners = useExtensionPoint(TOOL_VIEW_OPENER);
  const actions = toolCardActions(tool, allActions);
  const viewOpener = toolCardViewOpener(tool, allViewOpeners);
  const onOpenView = viewOpener
    ? () => {
        void Promise.resolve(viewOpener.open(tool)).catch((err) => {
          const owner = lookupToolViewOpenerOwner(viewOpener.id) ?? "unknown";
          console.error(`[plugin] tool view opener ${viewOpener.id} threw:`, err);
          reportPluginError(owner, "command", err, `tool view opener: ${viewOpener.id}`);
        });
      }
    : undefined;

  const actionClass = cn(
    "grid h-6 w-6 shrink-0 place-items-center rounded-md border-0 transition-[opacity,color,background-color]",
    model.isError
      ? "bg-canvas text-fg-muted opacity-100 shadow-[var(--shadow-control)] hover:text-fg"
      : "bg-transparent text-fg-faint opacity-0 group-hover:opacity-100 hover:bg-fg/[0.06] hover:text-fg",
  );

  return (
    <div data-slot="tool-card-root" className="group relative my-1">
      <div
        className={cn(
          "overflow-hidden rounded-[12px] transition-colors duration-150",
          model.isError ? "bg-negative/10" : model.needsAction ? "bg-warning/10" : "bg-surface",
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
            model.isError
              ? "hover:bg-negative/[0.06]"
              : model.needsAction
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
              title={model.intent.label}
              className={cn(
                "truncate text-[13px] font-medium leading-[1.3]",
                model.isError
                  ? "text-negative"
                  : model.needsAction
                    ? "text-warning"
                    : model.running
                      ? "text-accent"
                      : "text-fg",
              )}
            >
              {model.intent.label}
            </span>
            {model.detail && (
              <span
                title={model.detail}
                className={cn(
                  "font-mono text-[12px] leading-[1.4]",
                  model.isError ? "break-words text-negative/80" : "truncate text-fg-muted",
                )}
              >
                {model.detail}
              </span>
            )}
          </div>

          {/* Trailing meta pills — inline status counts (+N / -N / matches …). */}
          <ToolMeta items={model.metaItems} running={model.running} />

          {/* Running indicator — accent pulse dot. */}
          {model.running && <StatusDot tone="running" className="ml-0.5" />}

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
  const shown = visibleToolMetaItems(items, running);
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
