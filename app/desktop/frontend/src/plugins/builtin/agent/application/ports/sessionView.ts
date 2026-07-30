import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { AgentRunStartOptions } from "@/plugins/sdk";
import type { Item, StateSnapshot } from "@/rpc";
import type { AgentInput } from "../../domain/input";
import type { ApprovalDecision, RememberScope } from "../../domain/hitl";
import type { WireDecision } from "../hitl/wireDecision";
import type {
  AgentProblem,
  AgentSessionView,
  Message,
  PlanItem,
  RunUsage,
  TimelineEntry,
  ToolCall,
} from "@/plugins/sdk/types/agentSessionView";

export type ResolvePatch = {
  decision?: ApprovalDecision;
  answered?: boolean;
  answers?: Record<string, string[]>;
};

export type StopFn = (() => void) | null;
export type SendFn = ((input: AgentInput, options?: AgentRunStartOptions) => void) | null;
export type InterruptResumePayload =
  | {
      type: "approval";
      decision: WireDecision;
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
      runId: string,
      responses: InterruptResumeInput[],
      onSettled?: () => void,
      onStartError?: () => void,
    ) => void)
  | null;

export interface AgentSessionViewEntry {
  view: AgentSessionView;
  viewEpoch: number;
  stop: StopFn;
  send: SendFn;
  resume: ResumeFn;
}

export interface AgentSessionViewPort {
  useCurrentRootRunning(): boolean;
  useCurrentRootRunId(): string | null;
  useCurrentRootSegmentId(): string | null;
  useCurrentRootPlan(): PlanItem[];
  useToolCalls(): Record<string, ToolCall>;
  useSessionTimeline(): TimelineEntry[];
  useCurrentRootMessages(): Message[];
  useProblem(): AgentProblem | null;
  useSharedState<T = unknown>(path?: string): T | undefined;
  useCurrentRootUsage(): RunUsage;
  useCurrentRootContextTokens(): number | undefined;
  useAction(kind: "stop"): StopFn;
  useAction(kind: "send"): SendFn;
  getCurrentView(): AgentSessionView;
  getSessions(): Record<string, AgentSessionViewEntry>;
  getSession(sessionId: string): AgentSessionViewEntry | undefined;
  sendToSession(sessionId: string, input: AgentInput, options?: AgentRunStartOptions): boolean;
  dropMessage(sessionId: string, messageId: string): void;
  appendLocalUserMessage(sessionId: string, messageId: string, input: AgentInput): void;
  resetView(sessionId: string): void;
  applyCompletedItems(sessionId: string, items: Item[]): void;
  applyStateSnapshot(sessionId: string, snapshot: StateSnapshot): void;
  clearProblem(sessionId: string): void;
  resolveInterrupt(sessionId: string, itemId: string, settled: ResolvePatch): void;
  subscribeSessions(
    onChange: (sessions: Record<string, AgentSessionViewEntry>) => void,
  ): () => void;
}

const port = createSingletonPort<AgentSessionViewPort>("Agent session view port is not configured");

export const configureAgentSessionViewPort = port.configure;
export const agentSessionView = port.get;
