// Activity row — renders one agent tool invocation as a compact inline row
// (craft-aligned). Collapsed by default: a single ~28px line with chevron,
// status icon, label, and meta badges. Expands inline to show the plugin-
// contributed preview (or ToolInspector fallback). Selected state drives the
// inspector pane via the existing sessionStore wiring.
//
// This replaces the previous card-based ToolCard with a flat activity-row
// pattern: no border, no surface bg by default, the row sits directly in the
// message flow like structured text.
import * as React from "react";
import type { IconName } from "@/components/common";
import type { ToolCall } from "@/protocol/run/viewState";
import { Collapsible, Icon } from "@/components/common";
import { toolIntent, toolMetaItems, type ToolMetaItem } from "@/lib/agent/toolPresentation";
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
  expanded: boolean;
  onToggleExpand: () => void;
  onOpenView?: () => void;
}

export function ToolCard({ tool, expanded, onToggleExpand, onOpenView }: Props) {
  const running = tool.status === "running";
  const actions = useExtensionPoint(TOOL_ACTION).filter((a) => !a.predicate || a.predicate(tool));
  const intent = toolIntent(tool);
  const metaItems = toolMetaItems(tool);

  return (
    <div data-slot="tool-card-root" className="group relative my-0.5">
      {/* Collapsed / expanded row — a single bare text line, no bg, no
          border, no surface. Reads like a log entry on the canvas. */}
      <button
        data-slot="tool-card-trigger"
        type="button"
        aria-expanded={expanded}
        onClick={onToggleExpand}
        className={cn(
          "flex w-full items-center gap-2 px-2 py-0.5 text-left",
          "focus-visible:outline-none focus-visible:shadow-[0_0_0_2px_var(--color-accent)]",
        )}
      >
        {/* Chevron — muted, rotates on expand. No chevron-right in the
            icon set, so rotate chevron-down -90° for the closed state. */}
        <Icon
          name="chevron-down"
          size={12}
          className={cn(
            "shrink-0 text-fg-faint transition-transform duration-150",
            !expanded && "-rotate-90",
          )}
        />

        {/* Status icon — spinner/dot/check/x depending on state. */}
        <StatusIcon status={tool.status} tool={tool} />

        {/* Label + detail — one line, truncate overflow. */}
        <div className="flex min-w-0 flex-1 items-baseline gap-1.5">
          <span
            title={intent.label}
            className={cn(
              "truncate text-[13px] font-medium",
              running ? "text-accent" : "text-fg-muted",
              intent.detail && "shrink-0",
            )}
          >
            {intent.label}
          </span>
          {intent.detail && (
            <span
              title={intent.detail}
              className="min-w-0 truncate font-mono text-[12px] text-fg-faint"
            >
              {intent.detail}
            </span>
          )}
        </div>

        {/* Meta badges — inline, muted, separated by middle dots. */}
        <ToolMeta items={metaItems} />

        {/* Plugin actions — hover-reveal, quiet. */}
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
      </button>

      {/* Inline error reason — shown even when collapsed so failures are visible. */}
      {tool.status === "err" && tool.error && (
        <div className="pl-7 pr-2 pb-1 font-mono text-[11px] leading-snug text-negative">
          {tool.error}
        </div>
      )}

      {/* Expanded inline preview — plain text/code under the row, zero
          chrome (no card, no surface, no action buttons). */}
      <Collapsible open={expanded}>
        <div data-slot="tool-card-content" className="pl-6 pr-2 pb-1">
          <ToolPreview tool={tool} onOpenView={onOpenView} />
        </div>
      </Collapsible>
    </div>
  );
}

function StatusIcon({ status, tool }: { status: ToolCall["status"]; tool: ToolCall }) {
  if (status === "running") {
    return (
      <span
        data-slot="tool-card-status"
        className="inline-flex h-4 w-4 shrink-0 items-center justify-center"
      >
        <span className="h-2 w-2 rounded-full bg-accent shadow-[0_0_6px_var(--color-accent)] animate-pulse-dot" />
      </span>
    );
  }
  if (status === "err") {
    return (
      <span
        data-slot="tool-card-status"
        className="inline-flex h-4 w-4 shrink-0 items-center justify-center"
      >
        <Icon name="x" size={13} className="shrink-0 text-negative" />
      </span>
    );
  }
  if (status === "denied") {
    return (
      <span
        data-slot="tool-card-status"
        className="inline-flex h-4 w-4 shrink-0 items-center justify-center"
      >
        <Icon name="stop" size={12} className="shrink-0 text-fg-faint" />
      </span>
    );
  }
  if (status === "requires-action") {
    return (
      <span
        data-slot="tool-card-status"
        className="inline-flex h-4 w-4 shrink-0 items-center justify-center"
      >
        <Icon name="alert" size={13} className="shrink-0 text-warning" />
      </span>
    );
  }
  // ok — show the tool-type icon, not a generic check, so the row reads
  // differently per tool at a glance.
  const icon = toolIconFor(toolRoutingKey(tool));
  return (
    <span
      data-slot="tool-card-status"
      className="inline-flex h-4 w-4 shrink-0 items-center justify-center"
    >
      <Icon name={icon} size={13} className="shrink-0 text-fg-muted" />
    </span>
  );
}

const ACTION_BTN =
  "grid h-5 w-5 shrink-0 place-items-center rounded border-0 bg-transparent text-fg-faint opacity-0 transition-all group-hover:opacity-100 hover:text-fg hover:bg-fg/[0.05]";

function ToolMeta({ items }: { items: ToolMetaItem[] }) {
  if (items.length === 0) return null;

  return (
    <div className="hidden shrink-0 items-center gap-1.5 font-mono text-[11px] text-fg-faint tracking-normal normal-case sm:flex">
      {items.map((item, i) => (
        <React.Fragment key={item.id}>
          {i > 0 && <span className="text-fg-faint/50">·</span>}
          <span className={toolMetaToneClass(item.tone)}>{item.label}</span>
        </React.Fragment>
      ))}
    </div>
  );
}

function toolMetaToneClass(tone: ToolMetaItem["tone"]): string {
  if (tone === "success") return "text-success";
  if (tone === "negative") return "text-negative";
  return "";
}
