import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { AgentRunStartOptions } from "@/plugins/sdk";
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
import type {
  AgentRootAttention,
  AgentRunTreeNode,
  DelegatedRunNarrativesByItemId,
} from "../view/runTree";

export type ResolvePatch = {
  decision?: ApprovalDecision;
  answered?: boolean;
  answers?: Record<string, string[]>;
};

export type StopCurrentRootRunAction = () => boolean;
export type SynchronizeSessionAction = () => void;
export type CancelRunAction = (runId: string) => void;
export type SendAgentInputAction = (input: AgentInput, options?: AgentRunStartOptions) => void;
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
export type ResumeRunAction = (
  runId: string,
  responses: InterruptResumeInput[],
  onSettled?: () => void,
  onStartError?: () => void,
) => void;

export interface AgentSessionViewEntry {
  view: AgentSessionView;
  viewEpoch: number;
  viewRevision: number;
  stop: StopCurrentRootRunAction | null;
  send: SendAgentInputAction | null;
  resume: ResumeRunAction | null;
  synchronize: SynchronizeSessionAction | null;
  cancelRun: CancelRunAction | null;
}

export interface AgentViewRefreshToken {
  requestSequence: number;
  viewRevision: number;
}

export interface AgentSessionViewPort {
  useCurrentRootAttention(): AgentRootAttention;
  useCurrentRootRunId(): string | null;
  useCurrentRootSegmentId(): string | null;
  useCurrentRootPlan(): PlanItem[];
  useToolCalls(): Record<string, ToolCall>;
  useSessionTimeline(): TimelineEntry[];
  useRootNarrativeMessages(): Message[];
  useDelegatedRunNarratives(): DelegatedRunNarrativesByItemId;
  useRunTree(): AgentRunTreeNode[];
  useProblem(): AgentProblem | null;
  useSharedState<T = unknown>(path?: string): T | undefined;
  useCurrentRootUsage(): RunUsage;
  useCurrentRootContextTokens(): number | undefined;
  useAction(kind: "stop"): StopCurrentRootRunAction | null;
  useAction(kind: "send"): SendAgentInputAction | null;
  getCurrentView(): AgentSessionView;
  getSessions(): Record<string, AgentSessionViewEntry>;
  getSession(sessionId: string): AgentSessionViewEntry | undefined;
  sendToSession(sessionId: string, input: AgentInput, options?: AgentRunStartOptions): boolean;
  dropMessage(sessionId: string, messageId: string): void;
  appendLocalUserMessage(sessionId: string, messageId: string, input: AgentInput): void;
  beginViewRefresh(
    sessionId: string,
    invalidateQueuedRunEvents: boolean,
  ): AgentViewRefreshToken | null;
  commitViewRefresh(
    sessionId: string,
    token: AgentViewRefreshToken,
    view: AgentSessionView,
  ): boolean;
  clearProblem(sessionId: string): void;
  resolveInterrupt(sessionId: string, itemId: string, settled: ResolvePatch): void;
  subscribeSessions(
    onChange: (sessions: Record<string, AgentSessionViewEntry>) => void,
  ): () => void;
}

const port = createSingletonPort<AgentSessionViewPort>("Agent session view port is not configured");

export const configureAgentSessionViewPort = port.configure;
export const agentSessionView = port.get;
