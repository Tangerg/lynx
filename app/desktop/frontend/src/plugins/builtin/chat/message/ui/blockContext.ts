import type { PlanItem, ToolCall } from "@/plugins/builtin/agent/public/viewState";
import type { DelegatedRunNarrativesByItemId } from "@/plugins/builtin/agent/public/conversation";

/**
 * Per-render data for message blocks. Domain facts stay source-owned; only
 * workspace-owned disclosure state and presentation preferences join them.
 */
export interface BlockCtx {
  plan: PlanItem[];
  toolCalls: Record<string, ToolCall>;
  delegatedRunsByItemId: DelegatedRunNarrativesByItemId;
  onSelectTool: (id: string) => void;
  expandedIds: Set<string>;
  onToggleExpand: (id: string) => void;
  /** User-authored text renders immediately instead of replaying to its author. */
  instant?: boolean;
  /** Global streamed-text reveal preference. */
  typewriter?: boolean;
}
