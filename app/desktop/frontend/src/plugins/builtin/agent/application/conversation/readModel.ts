import type { Message, TimelineEntry, ToolCall } from "@/plugins/sdk/types/agentSessionView";
import { agentSessionView } from "../ports/sessionView";
import { selectRootNarrativeMessages } from "../view/runTree";
import type { TranscriptRow } from "./transcriptRows";

interface ActiveConversationSnapshot {
  messages: Message[];
  timeline: TimelineEntry[];
  toolCalls: Record<string, ToolCall>;
}

/** The turns alone — for consumers that navigate the transcript rather than render it. */
export function useActiveConversationMessages(): Message[] {
  return agentSessionView().useRootNarrativeMessages();
}

/**
 * The transcript as rows, each carrying only the session facts that row shows.
 *
 * What the renderer consumes. The narrowing is load-bearing, not tidiness — see
 * `TurnFacts`.
 */
export function useActiveConversationRows(): readonly TranscriptRow[] {
  return agentSessionView().useTranscriptRows();
}

export function getActiveConversationSnapshot(): ActiveConversationSnapshot {
  const view = agentSessionView().getCurrentView();
  return {
    messages: selectRootNarrativeMessages(view),
    timeline: view.timeline,
    toolCalls: view.toolCalls,
  };
}
