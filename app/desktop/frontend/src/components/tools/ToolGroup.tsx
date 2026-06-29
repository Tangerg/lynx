import { useState } from "react";
import type { ToolCall } from "@/protocol/run/viewState";
import { Collapsible, Icon } from "@/components/common";
import { cn } from "@/lib/utils";
import { hasToolView, openViewForTool } from "@/state/toolRouting";
import { ToolCard } from "./ToolCard";

const READONLY_TOOLS = new Set(["read", "grep", "glob", "lsp"]);

/**
 * Read-only tools — safe to fold into a collapsed summary group because they
 * have no side effects, so the user rarely needs to inspect each one. Names are
 * the wire `ToolInvocation.name` the runtime emits (see the tool-preview
 * registry keys in chat/tools/previews). Conservative on purpose:
 * shell/edit/write/skill/task can mutate state, so each always renders as
 * its own row even when adjacent — only `read` / `grep` / `glob` / `lsp` /
 * `lsp_diagnostics` group.
 */
export function isReadOnlyTool(name: string): boolean {
  return READONLY_TOOLS.has(name) || name.startsWith("lsp_");
}

interface Props {
  tools: ToolCall[];
  selectedToolId: string;
  onSelectTool: (id: string) => void;
  expandedIds: Set<string>;
  onToggleExpand: (id: string) => void;
}

// Terse, mono-friendly count by bucket: "3 read · 2 search · 1 lookup". Buckets
// stay accurate to the wire names (lsp_* is a lookup, not a search).
function summarize(tools: ToolCall[]): string {
  let read = 0;
  let search = 0;
  let lookup = 0;
  for (const t of tools) {
    if (t.name === "read") read++;
    else if (t.name === "lsp" || t.name.startsWith("lsp_")) lookup++;
    else search++; // grep / glob
  }
  const parts: string[] = [];
  if (read) parts.push(`${read} read`);
  if (search) parts.push(`${search} search`);
  if (lookup) parts.push(`${lookup} lookup`);
  return parts.join(" · ");
}

/**
 * A run of adjacent read-only tool calls, folded into one collapsible summary
 * row so a long agent turn stays scannable. Auto-expands while any child is
 * still running or has errored, then settles closed once they finish — unless
 * the user has pinned it open or closed. The group is a quiet vertical stack:
 * a summary row + indented child activity rows, no enclosing card.
 */
export function ToolGroup({
  tools,
  selectedToolId,
  onSelectTool,
  expandedIds,
  onToggleExpand,
}: Props) {
  const needsAttention = tools.some((t) => t.status === "running" || t.status === "err");
  // null = follow `needsAttention` (open while live/errored, closed when done);
  // a boolean = the user has pinned it that way.
  const [pinned, setPinned] = useState<boolean | null>(null);
  const expanded = pinned ?? needsAttention;

  return (
    <div className="my-0.5">
      {/* Summary row — craft-style inline row, no card. */}
      <button
        type="button"
        onClick={() => setPinned(!expanded)}
        aria-expanded={expanded}
        className={cn(
          "flex w-full items-center gap-2 rounded-md px-2 py-1 text-left transition-[background-color] duration-75",
          "hover:bg-fg/[0.02] focus-visible:outline-none focus-visible:shadow-[0_0_0_2px_var(--color-accent)]",
        )}
      >
        <Icon
          name="chevron-down"
          size={12}
          className={cn(
            "shrink-0 text-fg-faint transition-transform duration-150",
            !expanded && "-rotate-90",
          )}
        />
        <Icon name="search" size={13} className="shrink-0 text-fg-muted" />
        <span className="truncate text-[13px] font-medium text-fg-muted">
          {summarize(tools)}
        </span>
        <span className="ml-auto shrink-0 font-mono text-[11px] text-fg-faint">
          {tools.length} calls
        </span>
      </button>

      <Collapsible open={expanded}>
        <div className="pl-4">
          {tools.map((t) => (
            <ToolCard
              key={t.id}
              tool={t}
              selected={selectedToolId === t.id}
              expanded={expandedIds.has(t.id)}
              onToggleExpand={() => {
                onSelectTool(t.id);
                onToggleExpand(t.id);
              }}
              onOpenView={hasToolView(t) ? () => openViewForTool(t.id) : undefined}
            />
          ))}
        </div>
      </Collapsible>
    </div>
  );
}
