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

/**
 * `superseded` — the turn has started answering, so this group is the account of how
 * it got there rather than the thing in flight. Auto-open is for the live wave only;
 * a pin still wins, because a user who opened a group meant it.
 *
 * A failed child no longer forces it open either. That was the affordance for "you
 * need to see this", and the failure now carries its own flagged edge on the row —
 * visible closed, which is what makes collapsing safe.
 */
export function toolGroupModel(
  t: Translate,
  tools: readonly ToolCall[],
  pinned: ToolGroupPinnedState,
  superseded = false,
): ToolGroupModel {
  const needsAttention = toolGroupNeedsAttention(tools);
  const expanded = pinned ?? (needsAttention && !superseded);
  return {
    summary: summarizeToolGroup(t, tools),
    count: tools.length,
    needsAttention,
    expanded,
    nextPinned: !expanded,
  };
}
