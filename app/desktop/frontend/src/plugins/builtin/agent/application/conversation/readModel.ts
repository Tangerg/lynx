import type {
  Message,
  PlanItem,
  TimelineEntry,
  ToolCall,
} from "@/plugins/sdk/types/agentSessionView";
import { agentSessionView } from "../ports/sessionView";
import { selectCurrentRootMessages, selectCurrentRootPlan } from "../view/runTree";

interface ActiveConversationSnapshot {
  messages: Message[];
  plan: PlanItem[];
  timeline: TimelineEntry[];
  toolCalls: Record<string, ToolCall>;
}

export function useActiveConversationMessages(): Message[] {
  return agentSessionView().useCurrentRootMessages();
}

export function getActiveConversationSnapshot(): ActiveConversationSnapshot {
  const view = agentSessionView().getCurrentView();
  return {
    messages: selectCurrentRootMessages(view),
    plan: selectCurrentRootPlan(view),
    timeline: view.timeline,
    toolCalls: view.toolCalls,
  };
}
