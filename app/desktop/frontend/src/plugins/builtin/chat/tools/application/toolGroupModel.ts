import type { Translate } from "@/lib/i18n";
import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import {
  summarizeToolGroup,
  toolGroupNeedsAttention,
} from "@/plugins/builtin/agent/public/messagePresentation";

export type ToolGroupPinnedState = boolean | null;

export interface ToolGroupModel {
  summary: string;
  count: number;
  needsAttention: boolean;
  expanded: boolean;
  nextPinned: boolean;
}

export function toolGroupModel(
  t: Translate,
  tools: readonly ToolCall[],
  pinned: ToolGroupPinnedState,
): ToolGroupModel {
  const needsAttention = toolGroupNeedsAttention(tools);
  const expanded = pinned ?? needsAttention;
  return {
    summary: summarizeToolGroup(t, tools),
    count: tools.length,
    needsAttention,
    expanded,
    nextPinned: !expanded,
  };
}
