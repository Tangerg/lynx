import type { Item, RunRef } from "@/rpc";
import type { AgentSessionSnapshot } from "@/plugins/builtin/agent/application/ports/runtimeGateway";

export const VISUAL_AGENT_STATES = [
  "empty",
  "idle",
  "running",
  "waiting",
  "terminal",
  "error",
  "delegated",
  "long-content",
] as const;

export type VisualAgentState = (typeof VISUAL_AGENT_STATES)[number];

const SESSION_ID = "ses_visual";
const ROOT_RUN_ID = "run_root";
const CREATED_AT = "2026-07-31T08:00:00.000Z";
const METRICS = { steps: 4, activeDurationMs: 12_000 };
const PROFILE = { interruptTypes: ["approval"], requiredFeatures: ["subagents"] };

function run(status: RunRef["status"], patch: Partial<RunRef> = {}): RunRef {
  return {
    id: ROOT_RUN_ID,
    sessionId: SESSION_ID,
    status,
    createdAt: CREATED_AT,
    metrics: METRICS,
    protocolProfile: PROFILE,
    ...(status === "running" ? { activeSegmentId: "seg_root" } : {}),
    ...patch,
  };
}

function message(
  type: "userMessage" | "agentMessage",
  id: string,
  text: string,
  runId = ROOT_RUN_ID,
): Item {
  return {
    type,
    id,
    runId,
    status: "completed",
    createdAt: CREATED_AT,
    content: [{ type: "text", text }],
  };
}

const PROMPT = message(
  "userMessage",
  "item_prompt",
  "Review the Runtime boundary and keep application-owned atomicity out of the Agent Framework.",
);

const RESPONSE = message(
  "agentMessage",
  "item_response",
  "The boundary is clean: the framework exposes execution primitives, while the application owns persistence, idempotency, and transaction policy.",
);

const LONG_RESPONSE = message(
  "agentMessage",
  "item_long_response",
  [
    "## Architecture review",
    "",
    "The consumer owns persistence policy and transaction scope. The Agent Framework remains reusable because it exposes execution capability without importing application records.",
    "",
    "- keep Run and Item protocol facts at the application boundary;",
    "- keep framework identities opaque;",
    "- project durable state atomically before publishing it;",
    "- reject compatibility aliases during development.",
    "",
    "```go",
    "type Executor interface {",
    "    Execute(context.Context, Request) (Result, error)",
    "}",
    "```",
    "",
    "A deliberately long final paragraph verifies wrapping, reading measure, CJK fallback（中文混排）, inline code such as `expectedRevision`, and uninterrupted vertical rhythm without inventing a fixture-only message shape.",
  ].join("\n"),
);

const BASE: AgentSessionSnapshot = {
  runs: [],
  items: [],
  pendingInterruptSets: [],
};

export const AGENT_SESSION_SNAPSHOTS: Readonly<Record<VisualAgentState, AgentSessionSnapshot>> = {
  empty: BASE,
  idle: {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:00:12.000Z",
        outcome: { type: "completed" },
      }),
    ],
    items: [PROMPT, RESPONSE],
    pendingInterruptSets: [],
  },
  running: {
    runs: [run("running")],
    items: [PROMPT, RESPONSE],
    pendingInterruptSets: [],
    state: {
      type: "todos",
      sessionId: SESSION_ID,
      revision: 3,
      updatedAt: "2026-07-31T08:00:08.000Z",
      todos: [
        { id: "todo_boundary", text: "Verify boundary ownership", status: "completed" },
        { id: "todo_visual", text: "Review visual evidence", status: "in_progress" },
        { id: "todo_gates", text: "Run quality gates", status: "pending" },
      ],
    },
  },
  waiting: {
    runs: [run("waiting")],
    items: [PROMPT, RESPONSE],
    pendingInterruptSets: [
      {
        rootRunId: ROOT_RUN_ID,
        sessionId: SESSION_ID,
        createdAt: "2026-07-31T08:00:09.000Z",
        interrupts: [
          {
            type: "approval",
            itemId: "item_approval",
            runId: ROOT_RUN_ID,
            payload: {
              tool: {
                name: "shell",
                arguments: { command: "go test -race ./..." },
              },
              reason: "Run the race detector across the workspace before committing.",
              rememberable: true,
              risk: "medium",
            },
          },
        ],
      },
    ],
  },
  terminal: {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:00:12.000Z",
        outcome: { type: "canceled", detail: "Stopped after the requested review." },
      }),
    ],
    items: [PROMPT, RESPONSE],
    pendingInterruptSets: [],
  },
  error: {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:00:12.000Z",
        outcome: {
          type: "error",
          error: {
            type: "provider_rejected",
            detail: "The provider rejected the request. Verify the selected model and retry.",
          },
        },
      }),
    ],
    items: [PROMPT],
    pendingInterruptSets: [],
  },
  delegated: {
    runs: [
      run("running"),
      {
        id: "run_child",
        sessionId: SESSION_ID,
        status: "waiting",
        createdAt: "2026-07-31T08:00:03.000Z",
        metrics: { steps: 2, activeDurationMs: 7_000 },
        protocolProfile: PROFILE,
        parentRunId: ROOT_RUN_ID,
        rootRunId: ROOT_RUN_ID,
        spawnedByItemId: "item_delegate",
      },
      {
        id: "run_nested",
        sessionId: SESSION_ID,
        status: "running",
        activeSegmentId: "seg_nested",
        createdAt: "2026-07-31T08:00:05.000Z",
        metrics: { steps: 1, activeDurationMs: 3_000 },
        protocolProfile: PROFILE,
        parentRunId: "run_child",
        rootRunId: ROOT_RUN_ID,
        spawnedByItemId: "item_nested_delegate",
      },
    ],
    items: [
      PROMPT,
      RESPONSE,
      {
        type: "toolCall",
        id: "item_delegate",
        runId: ROOT_RUN_ID,
        status: "completed",
        createdAt: "2026-07-31T08:00:02.000Z",
        tool: {
          name: "task",
          arguments: { description: "Audit Agent Framework ownership" },
        },
      },
      message(
        "agentMessage",
        "item_child_response",
        "The framework remains generic. I found no application persistence type in its production graph.",
        "run_child",
      ),
      {
        type: "toolCall",
        id: "item_nested_delegate",
        runId: "run_child",
        status: "completed",
        createdAt: "2026-07-31T08:00:04.000Z",
        tool: {
          name: "task",
          arguments: { description: "Verify package dependencies" },
        },
      },
      message(
        "agentMessage",
        "item_nested_response",
        "Package graph verification is still running.",
        "run_nested",
      ),
    ],
    pendingInterruptSets: [
      {
        rootRunId: ROOT_RUN_ID,
        sessionId: SESSION_ID,
        createdAt: "2026-07-31T08:00:06.000Z",
        interrupts: [
          {
            type: "approval",
            itemId: "item_child_approval",
            runId: "run_child",
            payload: {
              tool: { name: "shell", arguments: { command: "go list -deps ./..." } },
              reason: "Inspect the complete dependency graph.",
              risk: "low",
            },
          },
        ],
      },
    ],
  },
  "long-content": {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:00:12.000Z",
        outcome: { type: "completed" },
      }),
    ],
    items: [PROMPT, LONG_RESPONSE],
    pendingInterruptSets: [],
  },
};

export const VISUAL_SESSION_ID = SESSION_ID;
