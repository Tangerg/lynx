import {
  errorDetail,
  errorRetryAfterSeconds,
  type CancelRunResponse,
  type ContentBlock,
  type Interrupt,
  type Item,
  type ItemDelta,
  type PendingInterruptSet,
  type ProblemData,
  type Question,
  type RunEvent,
  type RunMetrics,
  type RunOutcome,
  type RunProgress,
  type RunRef,
  type SegmentOutcome,
  type ToolInvocation,
  type Usage,
} from "@/rpc";
import type {
  AgentCancelResult,
  AgentEventEnvelope,
  AgentInterrupt,
  AgentItem,
  AgentItemDelta,
  AgentMessagePart,
  AgentPendingInterruptSet,
  AgentQuestion,
  AgentRunFact,
  AgentSegmentOutcome,
  AgentToolInvocation,
} from "@/plugins/sdk";
import type {
  AgentProblem,
  AgentRunMetrics,
  AgentRunOutcome,
  AgentRunProgress,
  RunUsage,
} from "@/plugins/sdk/types/agentSessionView";
import { runtimePlan } from "./runtimePlan";

function runtimeUsage(usage?: Usage): RunUsage {
  return {
    inputTokens: usage?.inputTokens ?? 0,
    outputTokens: usage?.outputTokens ?? 0,
    cacheReadTokens: usage?.cacheReadTokens ?? 0,
    ...(usage?.costUsd !== undefined ? { costUsd: usage.costUsd } : {}),
  };
}

export function runtimeRunMetrics(metrics: RunMetrics): AgentRunMetrics {
  return {
    steps: metrics.steps,
    activeDurationMillis: metrics.activeDurationMillis,
    usage: runtimeUsage(metrics.usage),
  };
}

export function runtimeProblem(problem: ProblemData): AgentProblem {
  const message = errorDetail(problem);
  const retryAfterSeconds = errorRetryAfterSeconds(problem);
  return {
    code: problem.type,
    ...(message !== undefined ? { message } : {}),
    ...(retryAfterSeconds !== undefined ? { retryAfterSeconds } : {}),
  };
}

function runtimeRunOutcome(outcome: RunOutcome): AgentRunOutcome {
  switch (outcome.type) {
    case "completed":
      return { type: "completed" };
    case "timedOut":
    case "failed":
    case "lost":
      return { type: outcome.type, error: runtimeProblem(outcome.error) };
    case "maxSteps":
    case "maxBudget":
    case "canceled":
      return {
        type: outcome.type,
        ...(outcome.detail !== undefined ? { detail: outcome.detail } : {}),
      };
  }
}

export function runtimeRunFact(run: RunRef): AgentRunFact {
  if (!run.status) throw new Error(`agent.adapter.run.statusMissing:run=${run.id}`);
  if (!run.createdAt) throw new Error(`agent.adapter.run.createdAtMissing:run=${run.id}`);

  const child = run.spawnedByItemId !== undefined;
  if (child && (!run.parentRunId || !run.rootRunId)) {
    throw new Error(
      `agent.adapter.run.childLineageMissing:run=${run.id};parentRun=${run.parentRunId ?? "missing"};rootRun=${run.rootRunId ?? "missing"}`,
    );
  }
  if (!child && (run.parentRunId || run.rootRunId)) {
    throw new Error(
      `agent.adapter.run.rootLineagePresent:run=${run.id};parentRun=${run.parentRunId ?? "missing"};rootRun=${run.rootRunId ?? "missing"}`,
    );
  }
  if (run.status === "running" && !run.activeSegmentId) {
    throw new Error(`agent.adapter.run.activeSegmentMissing:run=${run.id}`);
  }
  if (run.status !== "running" && run.activeSegmentId) {
    throw new Error(
      `agent.adapter.run.unexpectedActiveSegment:run=${run.id};status=${run.status};segment=${run.activeSegmentId}`,
    );
  }
  if (run.status === "finished" && (!run.outcome || !run.finishedAt)) {
    throw new Error(
      `agent.adapter.run.terminalFactsMissing:run=${run.id};outcome=${run.outcome?.type ?? "missing"};finishedAt=${run.finishedAt ?? "missing"}`,
    );
  }
  if (run.status !== "finished" && (run.outcome || run.finishedAt)) {
    throw new Error(
      `agent.adapter.run.unexpectedTerminalFacts:run=${run.id};status=${run.status};outcome=${run.outcome?.type ?? "missing"};finishedAt=${run.finishedAt ?? "missing"}`,
    );
  }
  if ((run.provider === undefined) !== (run.model === undefined)) {
    throw new Error(
      `agent.adapter.run.modelSelectionIncomplete:run=${run.id};provider=${run.provider ?? "missing"};model=${run.model ?? "missing"}`,
    );
  }
  if (run.reasoningEffort !== undefined && run.provider === undefined) {
    throw new Error(`agent.adapter.run.reasoningWithoutModel:run=${run.id}`);
  }

  return {
    id: run.id,
    sessionId: run.sessionId,
    parentRunId: child ? run.parentRunId! : null,
    rootRunId: child ? run.rootRunId! : run.id,
    spawnedByItemId: run.spawnedByItemId ?? null,
    status: run.status,
    activeSegmentId: run.activeSegmentId ?? null,
    outcome: run.outcome ? runtimeRunOutcome(run.outcome) : null,
    ...(run.provider !== undefined && run.model !== undefined
      ? {
          modelSelection: {
            provider: run.provider,
            model: run.model,
            ...(run.reasoningEffort !== undefined ? { reasoningEffort: run.reasoningEffort } : {}),
          },
        }
      : {}),
    metrics: runtimeRunMetrics(run.metrics),
    ...(run.contextTokens !== undefined && run.contextTokens > 0
      ? { contextTokens: run.contextTokens }
      : {}),
    createdAt: run.createdAt,
    finishedAt: run.finishedAt ?? null,
  };
}

function runtimeContent(block: ContentBlock): AgentMessagePart {
  return block.type === "text"
    ? { type: "text", text: block.text }
    : { type: "image", data: block.data, mime: block.mime };
}

function runtimeQuestion(question: Question): AgentQuestion {
  return {
    fields: question.fields.map((field) =>
      field.type === "text"
        ? { type: "text", prompt: field.prompt, ...(field.header ? { header: field.header } : {}) }
        : {
            type: "choice",
            prompt: field.prompt,
            options: field.options.map((option) => ({ ...option })),
            ...(field.header ? { header: field.header } : {}),
            ...(field.allowCustom !== undefined ? { allowCustom: field.allowCustom } : {}),
            ...(field.multiple !== undefined ? { multiple: field.multiple } : {}),
          },
    ),
    ...(question.answers ? { answers: question.answers.map((answer) => [...answer]) } : {}),
  };
}

function runtimeTool(tool: ToolInvocation): AgentToolInvocation {
  return {
    name: tool.name,
    arguments: { ...tool.arguments },
    ...(tool.result !== undefined ? { result: tool.result } : {}),
  };
}

export function runtimeItem(item: Item): AgentItem {
  switch (item.type) {
    case "userMessage":
      return {
        type: "userMessage",
        id: item.id,
        runId: item.runId,
        status: item.status,
        createdAt: item.createdAt,
        ...(item.content ? { content: item.content.map(runtimeContent) } : {}),
      };
    case "agentMessage":
      return {
        type: "agentMessage",
        id: item.id,
        runId: item.runId,
        status: item.status,
        createdAt: item.createdAt,
        ...(item.phase ? { phase: item.phase } : {}),
        ...(item.content ? { content: item.content.map(runtimeContent) } : {}),
      };
    case "reasoning":
      return {
        type: item.type,
        id: item.id,
        runId: item.runId,
        status: item.status,
        createdAt: item.createdAt,
        ...(item.text !== undefined ? { text: item.text } : {}),
        ...(item.redacted !== undefined ? { redacted: item.redacted } : {}),
      };
    case "compaction":
      return {
        type: item.type,
        id: item.id,
        runId: item.runId,
        status: item.status,
        createdAt: item.createdAt,
        ...(item.summary !== undefined ? { summary: item.summary } : {}),
        ...(item.droppedMessages !== undefined ? { droppedMessages: item.droppedMessages } : {}),
      };
    case "question":
      return {
        type: item.type,
        id: item.id,
        runId: item.runId,
        status: item.status,
        createdAt: item.createdAt,
        ...(item.question ? { question: runtimeQuestion(item.question) } : {}),
      };
    case "toolCall":
      return {
        type: item.type,
        id: item.id,
        runId: item.runId,
        status: item.status,
        startedAt: item.startedAt,
        ...(item.finishedAt !== undefined ? { finishedAt: item.finishedAt } : {}),
        ...(item.durationMillis !== undefined ? { durationMillis: item.durationMillis } : {}),
        ...(item.safetyClass !== undefined ? { safetyClass: item.safetyClass } : {}),
        ...(item.approvalDecision === "approve"
          ? { approvalDecision: "approved" as const }
          : item.approvalDecision === "deny"
            ? { approvalDecision: "declined" as const }
            : {}),
        ...(item.error ? { error: runtimeProblem(item.error) } : {}),
        ...(item.tool ? { tool: runtimeTool(item.tool) } : {}),
      };
  }
}

function runtimeItemDelta(delta: ItemDelta): AgentItemDelta {
  return { ...delta };
}

export function runtimeInterrupt(interrupt: Interrupt): AgentInterrupt {
  switch (interrupt.type) {
    case "approval":
      return {
        type: "approval",
        itemId: interrupt.itemId,
        runId: interrupt.runId,
        payload: {
          ...(interrupt.payload.reason !== undefined ? { reason: interrupt.payload.reason } : {}),
          ...(interrupt.payload.rememberable !== undefined
            ? { rememberable: interrupt.payload.rememberable }
            : {}),
          tool: runtimeTool(interrupt.payload.tool),
        },
      };
    case "question":
      return {
        ...interrupt,
        payload: { question: runtimeQuestion(interrupt.payload.question) },
      };
  }
}

export function runtimePendingInterruptSet(set: PendingInterruptSet): AgentPendingInterruptSet {
  return { ...set, interrupts: set.interrupts.map(runtimeInterrupt) };
}

function runtimeProgress(progress: RunProgress): AgentRunProgress {
  return {
    ...(progress.step !== undefined ? { step: progress.step } : {}),
    ...(progress.activity !== undefined ? { activity: progress.activity } : {}),
    ...(progress.usage ? { usage: runtimeUsage(progress.usage) } : {}),
    ...(progress.contextTokens !== undefined ? { contextTokens: progress.contextTokens } : {}),
  };
}

function runtimeSegmentOutcome(outcome: SegmentOutcome): AgentSegmentOutcome {
  if (outcome.type === "interrupt") {
    return { type: "interrupt", interrupts: outcome.interrupts.map(runtimeInterrupt) };
  }
  if (outcome.type === "suspended") return { type: "suspended" };
  return runtimeRunOutcome(outcome);
}

export function runtimeAgentEvent(envelope: RunEvent): AgentEventEnvelope {
  const event = envelope.event;
  const mapped = (() => {
    switch (event.type) {
      case "segment.started":
        return { type: event.type, run: runtimeRunFact(event.run) } as const;
      case "segment.progress":
        return { type: event.type, progress: runtimeProgress(event.progress) } as const;
      case "segment.finished":
        return {
          type: event.type,
          metrics: runtimeRunMetrics(event.metrics),
          outcome: runtimeSegmentOutcome(event.outcome),
        } as const;
      case "item.started":
      case "item.completed":
        return { type: event.type, item: runtimeItem(event.item) } as const;
      case "item.delta":
        return {
          type: event.type,
          itemId: event.itemId,
          delta: runtimeItemDelta(event.delta),
        } as const;
      case "plan.updated":
        return { type: event.type, plan: runtimePlan(event.plan) } as const;
    }
  })();
  return { ...envelope, event: mapped };
}

export function runtimeCancelResult(response: CancelRunResponse): AgentCancelResult {
  return response.type === "root"
    ? { type: "root", run: runtimeRunFact(response.run) }
    : {
        type: "child",
        run: runtimeRunFact(response.run),
        rootRun: runtimeRunFact(response.rootRun),
      };
}
