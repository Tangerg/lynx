import type {
  Message,
  PlanItem,
  TimelineEntry,
  ToolCall,
} from "@/plugins/builtin/agent/public/viewState";
import { agentViewState } from "../ports/viewState";

interface ActiveConversationSnapshot {
  messages: Message[];
  plan: PlanItem[];
  timeline: TimelineEntry[];
  toolCalls: Record<string, ToolCall>;
}

export function useActiveConversationMessages(): Message[] {
  return agentViewState().useMessages();
}

export function getActiveConversationSnapshot(): ActiveConversationSnapshot {
  const view = agentViewState().getCurrentView();
  return {
    messages: view.messages,
    plan: view.plan,
    timeline: view.timeline,
    toolCalls: view.toolCalls,
  };
}
