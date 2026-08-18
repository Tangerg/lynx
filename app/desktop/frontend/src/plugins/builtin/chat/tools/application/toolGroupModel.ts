import type { Translate } from "@/lib/i18n";
import type { ToolCall } from "@/plugins/builtin/agent/public/viewState";
import {
  summarizeActivity,
  toolGroupNeedsAttention,
} from "@/plugins/builtin/agent/public/messagePresentation";

export type ToolGroupPinnedState = boolean | null;

export interface ToolGroupModel {
  summary: string;
  /** The tool the group is mostly made of; the renderer resolves its glyph through
   *  the same icon registry as a single row. */
  dominantTool: string;
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
    summary: summarizeActivity(t, tools),
    dominantTool: dominantTool(tools),
    count: tools.length,
    needsAttention,
    expanded,
    nextPinned: !expanded,
  };
}

/**
 * What the group is mostly made of.
 *
 * A tie goes to whichever came first, which is the tool the group opened with —
 * the same thing the summary counts. Empty is not a group the renderer produces,
 * but the type allows it, so it answers with nothing and the glyph falls back.
 */
function dominantTool(tools: readonly ToolCall[]): string {
  const counts = new Map<string, number>();
  for (const tool of tools) counts.set(tool.name, (counts.get(tool.name) ?? 0) + 1);
  let dominant: string | undefined;
  let best = 0;
  for (const [name, count] of counts) {
    if (count > best) {
      best = count;
      dominant = name;
    }
  }
  return dominant ?? "";
}
