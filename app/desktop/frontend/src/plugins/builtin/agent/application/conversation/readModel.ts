import type {
  Message,
  PlanItem,
  TimelineEntry,
  ToolCall,
} from "@/plugins/sdk/types/agentSessionView";
import { agentSessionView } from "../ports/sessionView";
import type { DelegatedRunNarrativesByItemId } from "../view/runTree";
import { selectCurrentRootPlan, selectRootNarrativeMessages } from "../view/runTree";

interface ActiveConversationSnapshot {
  messages: Message[];
  plan: PlanItem[];
  timeline: TimelineEntry[];
  toolCalls: Record<string, ToolCall>;
}

export function useActiveConversationMessages(): Message[] {
  return agentSessionView().useRootNarrativeMessages();
}

export function useDelegatedConversationRuns(): DelegatedRunNarrativesByItemId {
  return agentSessionView().useDelegatedRunNarratives();
}

export function getActiveConversationSnapshot(): ActiveConversationSnapshot {
  const view = agentSessionView().getCurrentView();
  return {
    messages: selectRootNarrativeMessages(view),
    plan: selectCurrentRootPlan(view),
    timeline: view.timeline,
    toolCalls: view.toolCalls,
  };
}
