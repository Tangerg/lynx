import type { AgentRunStartOptions } from "@/plugins/sdk";
import type { Item } from "@/rpc";
import type { AgentInput } from "../../domain/input";
import type { RememberScope } from "../../domain/hitl";
import type {
  AgentViewState,
  Message,
  PlanItem,
  RunError,
  RunUsage,
  TimelineEntry,
  ToolCall,
} from "@/plugins/builtin/agent/public/viewState";

export type ResolvePatch = {
  decision?: "approved" | "declined";
  answered?: boolean;
  answers?: Record<string, string[]>;
};

export type StopFn = (() => void) | null;
export type SendFn = ((input: AgentInput, options?: AgentRunStartOptions) => void) | null;
export type InterruptResumePayload =
  | {
      type: "approval";
      decision: "approve" | "deny";
      editedArgs?: Record<string, unknown>;
      remember?: { scope: RememberScope };
    }
  | {
      type: "answer";
      answers: Record<string, string[]>;
    };
export interface InterruptResumeInput {
  itemId: string;
  response: InterruptResumePayload;
}
export type ResumeFn =
  | ((
      parentRunId: string,
      responses: InterruptResumeInput[],
      onSettled?: () => void,
      onStartError?: () => void,
    ) => void)
  | null;

export interface AgentViewSession {
  view: AgentViewState;
  viewEpoch: number;
  stop: StopFn;
  send: SendFn;
  resume: ResumeFn;
}

export interface AgentViewStatePort {
  useRunning(): boolean;
  useRunId(): string | null;
  usePlan(): PlanItem[];
  useToolCalls(): Record<string, ToolCall>;
  useTimeline(): TimelineEntry[];
  useMessages(): Message[];
  useError(): RunError | null;
  useSharedState<T = unknown>(path?: string): T | undefined;
  useUsage(): RunUsage;
  useContextTokens(): number | undefined;
  useAction(kind: "stop"): StopFn;
  useAction(kind: "send"): SendFn;
  getCurrentView(): AgentViewState;
  getSessions(): Record<string, AgentViewSession>;
  getSession(sessionId: string): AgentViewSession | undefined;
  sendToSession(sessionId: string, input: AgentInput, options?: AgentRunStartOptions): boolean;
  dropMessage(sessionId: string, messageId: string): void;
  appendLocalUserMessage(sessionId: string, messageId: string, input: AgentInput): void;
  resetView(sessionId: string): void;
  applyCompletedItems(sessionId: string, items: Item[]): void;
  clearError(sessionId: string): void;
  resolveInterrupt(sessionId: string, itemId: string, settled: ResolvePatch): void;
  subscribeSessions(onChange: (sessions: Record<string, AgentViewSession>) => void): () => void;
}

let port: AgentViewStatePort | null = null;

export function configureAgentViewStatePort(next: AgentViewStatePort): void {
  port = next;
}

export function agentViewState(): AgentViewStatePort {
  if (!port) throw new Error("Agent view state port is not configured");
  return port;
}
