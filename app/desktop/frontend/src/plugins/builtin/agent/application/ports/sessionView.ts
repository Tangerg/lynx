import { createSingletonPort } from "@/lib/ports/singletonPort";
import type { AgentRunStartOptions } from "@/plugins/sdk";
import type { AgentInput } from "../../domain/input";
import type { ApprovalDecision, RememberScope } from "../../domain/hitl";
import type { WireDecision } from "../hitl/wireDecision";
import type {
  AgentProblem,
  AgentRunMetrics,
  AgentRunOutcome,
  AgentSessionView,
  Message,
  TimelineEntry,
  ToolCall,
} from "@/plugins/sdk/types/agentSessionView";
import type { AgentRootAttention, AgentRunTreeNode } from "../view/runTree";
import type { TranscriptRow } from "../conversation/transcriptRows";

export type ResolvePatch = {
  decision?: ApprovalDecision;
  answered?: boolean;
  answers?: string[][];
};

export type StopCurrentRootRunAction = () => boolean;
export type SessionProjectionSynchronizationOwnership =
  "after-live" | "replace-live" | "retire-live";
/** Request the mounted Session's single projection owner to reconcile durable
 * facts. True means an authoritative snapshot committed; false means the read
 * was superseded, unavailable, or failed and the caller may retry. */
export type SynchronizeSessionAction = (
  ownership?: SessionProjectionSynchronizationOwnership,
) => Promise<boolean>;
export type CancelRunAction = (runId: string) => void;
export type SendAgentInputAction = (input: AgentInput, options?: AgentRunStartOptions) => boolean;
export type InterruptResumePayload =
  | {
      type: "approval";
      decision: WireDecision;
      editedArgs?: Record<string, unknown>;
      remember?: { scope: RememberScope };
    }
  | {
      type: "answer";
      answers: string[][];
    };
export interface InterruptResumeInput {
  itemId: string;
  response: InterruptResumePayload;
}
export type ResumeRunAction = (
  runId: string,
  responses: InterruptResumeInput[],
  onSettled?: () => void,
  /** Return true when a newer authoritative projection already superseded the
   *  rejected opening, so the Adapter must not publish a stale command error. */
  onStartError?: () => boolean | void,
) => boolean;

export interface AgentSessionViewEntry {
  view: AgentSessionView;
  viewEpoch: number;
  viewRevision: number;
  /** Monotonic commits of durable authoritative projections. Unlike
   * `viewRevision`, live events and optimistic writes do not advance it. */
  authoritativeRevision: number;
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
  useCurrentRootOutcome(): AgentRunOutcome | null;
  useCurrentRootMetrics(): AgentRunMetrics | null;
  useToolCalls(): Record<string, ToolCall>;
  useSessionTimeline(): TimelineEntry[];
  useRootNarrativeMessages(): Message[];
  /** The transcript as rows, each holding only the session facts it renders. */
  useTranscriptRows(): readonly TranscriptRow[];
  useRunTree(): AgentRunTreeNode[];
  useProblem(): AgentProblem | null;
  useSharedState<T = unknown>(path?: string): T | undefined;
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
  /** Revoke snapshot tokens and queued live-event cohorts without clearing the
   * currently visible material or starting a successor read. */
  retireProjectionGeneration(sessionIds: readonly string[]): void;
  clearProblem(sessionId: string): void;
  resolveInterrupt(
    sessionId: string,
    itemId: string,
    settled: ResolvePatch,
    resolvedAt: number,
  ): void;
  subscribeSessions(
    onChange: (sessions: Record<string, AgentSessionViewEntry>) => void,
  ): () => void;
}

const port = createSingletonPort<AgentSessionViewPort>("Agent session view port is not configured");

export const configureAgentSessionViewPort = port.configure;
export const agentSessionView = port.get;
