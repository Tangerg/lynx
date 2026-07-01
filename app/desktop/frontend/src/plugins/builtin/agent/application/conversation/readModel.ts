import type {
  AgentViewState,
  Message,
  PlanItem,
  TimelineEntry,
  ToolCall,
} from "@/plugins/sdk/types/agentView";
import {
  getCurrentSessionView,
  useAgentMessages,
} from "@/plugins/builtin/agent/adapters/agentStore";

interface ActiveConversationSnapshot {
  messages: Message[];
  plan: PlanItem[];
  timeline: TimelineEntry[];
  toolCalls: Record<string, ToolCall>;
}

export function useActiveConversationMessages(): Message[] {
  return useAgentMessages();
}

export function getActiveConversationSnapshot(): ActiveConversationSnapshot {
  const view: AgentViewState = getCurrentSessionView();
  return {
    messages: view.messages,
    plan: view.plan,
    timeline: view.timeline,
    toolCalls: view.toolCalls,
  };
}
