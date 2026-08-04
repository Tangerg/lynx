import type { Item, RunEvent, RunProtocolProfile, RunRef, StreamEvent } from "@/rpc";
import type { AgentSessionSnapshot } from "@/plugins/builtin/agent/application/ports/runtimeGateway";

export const VISUAL_AGENT_STATES = [
  "empty",
  "idle",
  "running",
  "steer",
  "waiting",
  "question",
  "terminal",
  "canceled",
  "error",
  "recovery",
  "delegated",
  "long-content",
  "narrative",
  "tool-shells",
  "waves",
] as const;

export type VisualAgentState = (typeof VISUAL_AGENT_STATES)[number];

const SESSION_ID = "ses_visual";
const ROOT_RUN_ID = "run_root";
const CREATED_AT = "2026-07-31T08:00:00.000Z";
const METRICS = { steps: 4, activeDurationMs: 12_000 };
const PROFILE: RunProtocolProfile = {
  interruptTypes: ["approval"],
  requiredFeatures: ["subagents"],
};

const SAFETY_CLASS: Record<string, "safe" | "write" | "exec" | "network"> = {
  read: "safe",
  grep: "safe",
  glob: "safe",
  lsp: "safe",
  delegate_task: "safe",
  edit: "write",
  write: "write",
  shell: "exec",
};

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

const RUNNING_RESPONSE: Item = {
  type: "agentMessage",
  id: "item_running_response",
  runId: ROOT_RUN_ID,
  status: "running",
  createdAt: CREATED_AT,
  content: [{ type: "text", text: "I’m tracing the ownership boundary and verifying" }],
};

const RUNNING_REASONING: Item = {
  type: "reasoning",
  id: "item_reasoning",
  runId: ROOT_RUN_ID,
  status: "running",
  createdAt: CREATED_AT,
  text: "The framework must expose execution capability without knowing the application’s persistence records.",
};

// A settled read ahead of the running one, so the two fold into a tool GROUP:
// a disclosure nested inside a disclosure, auto-open because a child is still
// working. That is the shape a real working turn spends most of its time in, and
// until now no fixture rendered it — which is how a nested row overflowing its
// parent's rounded corner shipped twice.
const RUNNING_READ: Item = {
  type: "toolCall",
  safetyClass: "safe",
  id: "item_running_read",
  runId: ROOT_RUN_ID,
  status: "completed",
  startedAt: CREATED_AT,
  tool: {
    name: "read",
    arguments: {
      path: "/Users/visual/lynx/app/runtime/internal/session/atomicity_and_idempotency.go",
    },
  },
};

const RUNNING_TOOL: Item = {
  type: "toolCall",
  safetyClass: "safe",
  id: "item_running_tool",
  runId: ROOT_RUN_ID,
  status: "running",
  startedAt: CREATED_AT,
  tool: {
    name: "grep",
    arguments: {
      pattern: "idempotency|atomicity",
      path: "app/runtime",
    },
  },
};

// One turn holding all three activity shells, because the shells are the whole of
// the differentiation between a glance, a product and trouble — and until this state
// existed no golden rendered a failed or a refused tool call at all. A read is a
// line, a command is a card, a failure and a refusal are flagged, and the two flags
// carry different tones on purpose: a refusal is a decision, not a fault.
const SHELL_READ: Item = {
  type: "toolCall",
  safetyClass: "safe",
  id: "item_shells_read",
  runId: ROOT_RUN_ID,
  status: "completed",
  startedAt: CREATED_AT,
  tool: { name: "read", arguments: { path: "app/runtime/internal/session/store.go" } },
};

const SHELL_COMMAND: Item = {
  type: "toolCall",
  safetyClass: "exec",
  id: "item_shells_command",
  runId: ROOT_RUN_ID,
  status: "completed",
  startedAt: CREATED_AT,
  tool: { name: "shell", arguments: { command: "go test ./internal/session/..." } },
};

const SHELL_FAILED: Item = {
  type: "toolCall",
  safetyClass: "write",
  id: "item_shells_failed",
  runId: ROOT_RUN_ID,
  status: "incomplete",
  startedAt: CREATED_AT,
  tool: { name: "edit", arguments: { path: "app/runtime/internal/session/store.go" } },
  error: { type: "tool_failed", detail: "store.go changed on disk after it was read." },
};

const SHELL_DENIED: Item = {
  type: "toolCall",
  safetyClass: "write",
  id: "item_shells_denied",
  runId: ROOT_RUN_ID,
  status: "incomplete",
  startedAt: CREATED_AT,
  tool: { name: "write", arguments: { path: ".env.production" } },
  error: { type: "denied_by_user", detail: "You declined this write." },
};

// A long turn: two rounds of work, each answered, then a third round still in flight.
// The two answered rounds fold to one row apiece; the live one stays open. Without a
// state shaped like this, nothing in the goldens ever showed the fold at all — and the
// fold is the whole reason a long turn stays readable.
const WAVE_REASONING_ONE: Item = {
  type: "reasoning",
  id: "item_w_reason_1",
  runId: ROOT_RUN_ID,
  status: "completed",
  createdAt: CREATED_AT,
  text: "Find where the boundary is enforced before changing anything.",
};

const WAVE_READ: Item = {
  type: "toolCall",
  safetyClass: "safe",
  id: "item_w_read",
  runId: ROOT_RUN_ID,
  status: "completed",
  startedAt: CREATED_AT,
  tool: { name: "read", arguments: { path: "app/runtime/internal/session/store.go" } },
};

const WAVE_GREP: Item = {
  type: "toolCall",
  safetyClass: "safe",
  id: "item_w_grep",
  runId: ROOT_RUN_ID,
  status: "completed",
  startedAt: CREATED_AT,
  tool: { name: "grep", arguments: { pattern: "RunInTx", path: "app/runtime" } },
};

const WAVE_ANSWER_ONE = message(
  "agentMessage",
  "item_w_answer_1",
  "The transaction seam is `RunInTx`, and every store call already goes through it.",
);

const WAVE_REASONING_TWO: Item = {
  type: "reasoning",
  id: "item_w_reason_2",
  runId: ROOT_RUN_ID,
  status: "completed",
  createdAt: CREATED_AT,
  text: "So the change is one call site, plus a test that proves the rollback.",
};

const WAVE_EDIT: Item = {
  type: "toolCall",
  safetyClass: "write",
  id: "item_w_edit",
  runId: ROOT_RUN_ID,
  status: "completed",
  startedAt: CREATED_AT,
  tool: { name: "edit", arguments: { path: "app/runtime/internal/session/store.go" } },
};

const WAVE_ANSWER_TWO = message(
  "agentMessage",
  "item_w_answer_2",
  "Done — the write now rolls back with the transaction.",
);

const WAVE_LIVE_REASONING: Item = {
  type: "reasoning",
  id: "item_w_reason_3",
  runId: ROOT_RUN_ID,
  status: "running",
  createdAt: CREATED_AT,
  text: "Now check whether the test suite covers the rollback path.",
};

const WAVE_LIVE_TOOL: Item = {
  type: "toolCall",
  safetyClass: "exec",
  id: "item_w_live",
  runId: ROOT_RUN_ID,
  status: "running",
  startedAt: CREATED_AT,
  tool: { name: "shell", arguments: { command: "go test ./internal/session/..." } },
};

function tailEvent(index: number, event: StreamEvent): RunEvent {
  return {
    event,
    eventId: `event_visual_${index}`,
    runId: ROOT_RUN_ID,
    segmentId: "seg_root",
    timestamp: CREATED_AT,
  };
}

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
    // A SECOND heading, so this answer has an outline and the end rail is
    // finally photographed at rest. One heading is the answer's own title and
    // the rail declines to draw it, which is why every golden until now framed
    // an empty end gutter — the one place a width regression there could hide.
    "### Where the boundary is enforced",
    "",
    "A deliberately long final paragraph verifies wrapping, reading measure, CJK fallback（中文混排）, inline code such as `expectedRevision`, and uninterrupted vertical rhythm without inventing a fixture-only message shape.",
  ].join("\n"),
);

// A multi-turn conversation with the full block vocabulary — the state the
// narrative rails and the block grammar are actually FOR. Every other fixture is
// one question and one answer, which is exactly the shape in which a turn map and
// an answer outline have nothing to say, so neither could be photographed.
const NARRATIVE_TURN_1 = message(
  "userMessage",
  "item_n_ask1",
  "Pull the retry logic out of checkout into its own hook, run the tests, then show me the tradeoffs.",
);

const NARRATIVE_REASONING: Item = {
  type: "reasoning",
  id: "item_n_reasoning",
  runId: ROOT_RUN_ID,
  status: "completed",
  createdAt: "2026-07-31T08:01:00.000Z",
  text: "The retry loop is inlined in handleSubmit with a hardcoded ceiling and no backoff. Extracting it has two hazards: the idempotency key has to survive the whole retry cycle, and the component may unmount mid-flight.",
};

function narrativeTool(
  id: string,
  name: string,
  args: Record<string, unknown>,
  result?: unknown,
): Item {
  return {
    type: "toolCall",
    id,
    runId: ROOT_RUN_ID,
    status: "completed",
    startedAt: "2026-07-31T08:01:04.000Z",
    safetyClass: SAFETY_CLASS[name] ?? "exec",
    tool: { name, arguments: args, ...(result === undefined ? {} : { result }) },
  };
}

const NARRATIVE_ANSWER_1 = message(
  "agentMessage",
  "item_n_answer1",
  [
    "## The extracted hook",
    "",
    "The key is pinning the idempotency key to a `useRef` so the whole retry cycle shares one — otherwise every attempt reads as a new order at the gateway.",
    "",
    "- **Exponential backoff**: 400ms → 800ms → 1600ms, three attempts;",
    "- **Cancellable**: aborts on unmount and stops writing state;",
    "- **Observable**: every failure reports a `payment.retry` event.",
    "",
    "## Strategy comparison",
    "",
    "| Strategy | P95 success | Double-charge risk | Cost |",
    "| --- | --- | --- | --- |",
    "| Fixed interval | 91.2% | High | Low |",
    "| Exponential backoff | 96.8% | Medium | Low |",
    "| Backoff + jitter + key | 98.4% | Low | Medium |",
    "| Server-side replay queue | 99.1% | Low | High |",
    "",
    "Sampled from the payment gateway over 2026-07, n = 41,208.",
  ].join("\n"),
);

const NARRATIVE_TURN_2 = message(
  "userMessage",
  "item_n_ask2",
  "There are still a few double charges in production. Check the errors and give me two fixes.",
);

const NARRATIVE_ANSWER_2 = message(
  "agentMessage",
  "item_n_answer2",
  [
    "## Two ways to fix it",
    "",
    "Both remove the double charge; they differ in which layer owns the key's lifetime.",
    "",
    "1. **Key follows the order** — mint it once at checkout and persist it, so a refresh reuses it. Two files, no backend work.",
    "2. **Server issues the key** — the checkout endpoint returns an intent id and the client only forwards it. Five files, one backend day.",
  ].join("\n"),
);

const NARRATIVE_TURN_3 = message(
  "userMessage",
  "item_n_ask3",
  "Go with the first one, and add a regression test for the refresh case.",
);

const NARRATIVE_COMPACTION: Item = {
  type: "compaction",
  id: "item_n_compaction",
  runId: ROOT_RUN_ID,
  status: "completed",
  createdAt: "2026-07-31T08:02:00.000Z",
  droppedMessages: 34,
  summary: "Earlier tool output folded into a summary.",
};

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
    items: [PROMPT],
    pendingInterruptSets: [],
    state: {
      type: "plan",
      sessionId: SESSION_ID,
      revision: 3,
      updatedAt: "2026-07-31T08:00:08.000Z",
      plan: [
        { id: "step_boundary", description: "Verify boundary ownership", status: "completed" },
        { id: "step_visual", description: "Review visual evidence", status: "in_progress" },
        { id: "step_gates", description: "Run quality gates", status: "pending" },
      ],
    },
  },
  steer: {
    runs: [run("running")],
    items: [PROMPT],
    pendingInterruptSets: [],
    state: {
      type: "plan",
      sessionId: SESSION_ID,
      revision: 3,
      updatedAt: "2026-07-31T08:00:08.000Z",
      plan: [
        { id: "step_boundary", description: "Verify boundary ownership", status: "completed" },
        { id: "step_visual", description: "Review visual evidence", status: "in_progress" },
        { id: "step_gates", description: "Run quality gates", status: "pending" },
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
  question: {
    runs: [run("waiting")],
    items: [PROMPT, RESPONSE],
    pendingInterruptSets: [
      {
        rootRunId: ROOT_RUN_ID,
        sessionId: SESSION_ID,
        createdAt: "2026-07-31T08:00:09.000Z",
        interrupts: [
          {
            type: "question",
            itemId: "item_question",
            runId: ROOT_RUN_ID,
            payload: {
              question: {
                fields: [
                  {
                    type: "choice",
                    header: "Gate",
                    prompt: "Which gate should run next?",
                    options: [
                      {
                        label: "Race detector",
                        description: "Exercise concurrency and cancellation paths.",
                      },
                      {
                        label: "Visual suite",
                        description: "Verify light, dark, long-content, and HITL states.",
                      },
                    ],
                  },
                ],
              },
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
        outcome: { type: "completed" },
      }),
    ],
    items: [PROMPT, RESPONSE],
    pendingInterruptSets: [],
  },
  canceled: {
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
  recovery: {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:00:12.000Z",
        outcome: {
          type: "error",
          error: {
            type: "run_lost",
            detail: "The Runtime restarted before this Run reached a terminal event.",
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
        startedAt: "2026-07-31T08:00:02.000Z",
        safetyClass: "safe",
        tool: {
          name: "delegate_task",
          arguments: { summary: "Audit Agent Framework ownership", instructions: "…" },
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
        startedAt: "2026-07-31T08:00:04.000Z",
        safetyClass: "safe",
        tool: {
          name: "delegate_task",
          arguments: { summary: "Verify package dependencies", instructions: "…" },
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
  narrative: {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:02:30.000Z",
        outcome: { type: "completed" },
      }),
    ],
    items: [
      NARRATIVE_TURN_1,
      NARRATIVE_REASONING,
      // Four adjacent read-only calls, so the transcript photographs a tool GROUP
      // — a disclosure nested inside a disclosure. Every defect this shape has
      // shipped (a row overflowing its parent's rounded corner, an inner rail
      // with nowhere to go) survived because no fixture rendered one.
      narrativeTool("item_n_read", "read", { path: "src/checkout/checkout.tsx" }),
      narrativeTool("item_n_read_2", "read", {
        file_path:
          "/Users/visual/lynx/app/desktop/frontend/src/plugins/builtin/chat/tools/ui/ToolGroup.tsx",
      }),
      narrativeTool("item_n_read_3", "read", { path: "src/checkout/api/pay.ts" }),
      narrativeTool("item_n_grep", "grep", { pattern: "retry|backoff", path: "src" }, "7 matches"),
      narrativeTool(
        "item_n_edit",
        "edit",
        { path: "src/checkout/hooks/useRetryPayment.ts" },
        "Created 85 lines",
      ),
      NARRATIVE_ANSWER_1,
      NARRATIVE_TURN_2,
      NARRATIVE_COMPACTION,
      narrativeTool("item_n_search", "web_search", { query: "stripe idempotency key retry" }),
      NARRATIVE_ANSWER_2,
      NARRATIVE_TURN_3,
    ],
    pendingInterruptSets: [
      {
        rootRunId: ROOT_RUN_ID,
        sessionId: SESSION_ID,
        createdAt: "2026-07-31T08:02:20.000Z",
        interrupts: [
          {
            type: "approval",
            itemId: "item_n_approval",
            runId: ROOT_RUN_ID,
            payload: {
              tool: {
                name: "shell",
                arguments: { command: "rm -rf node_modules .next && pnpm install" },
              },
              reason: "rm -rf deletes uncommitted build output and cannot be undone.",
              rememberable: true,
              risk: "high",
            },
          },
        ],
      },
    ],
  },
  "tool-shells": {
    runs: [
      run("finished", {
        finishedAt: "2026-07-31T08:00:12.000Z",
        outcome: { type: "completed" },
      }),
    ],
    items: [PROMPT, SHELL_READ, SHELL_COMMAND, SHELL_FAILED, SHELL_DENIED, RESPONSE],
    pendingInterruptSets: [],
  },

  waves: {
    runs: [run("running")],
    items: [
      PROMPT,
      WAVE_REASONING_ONE,
      WAVE_READ,
      WAVE_GREP,
      WAVE_ANSWER_ONE,
      WAVE_REASONING_TWO,
      WAVE_EDIT,
      WAVE_ANSWER_TWO,
    ],
    pendingInterruptSets: [],
  },
};

export const AGENT_SESSION_TAIL_EVENTS: Readonly<Record<VisualAgentState, RunEvent[]>> = {
  empty: [],
  idle: [],
  running: [
    tailEvent(1, { type: "item.started", item: RUNNING_REASONING }),
    tailEvent(3, { type: "item.started", item: RUNNING_READ }),
    tailEvent(4, { type: "item.started", item: RUNNING_TOOL }),
    tailEvent(5, { type: "item.started", item: RUNNING_RESPONSE }),
  ],
  steer: [
    tailEvent(1, { type: "item.started", item: RUNNING_REASONING }),
    tailEvent(3, { type: "item.started", item: RUNNING_READ }),
    tailEvent(4, { type: "item.started", item: RUNNING_TOOL }),
    tailEvent(5, { type: "item.started", item: RUNNING_RESPONSE }),
  ],
  waiting: [],
  question: [],
  terminal: [],
  canceled: [],
  error: [],
  recovery: [],
  delegated: [],
  "long-content": [],
  narrative: [],
  "tool-shells": [],
  // The live round arrives as started items, not as snapshot history: a snapshot holds
  // only what has reached a terminal state.
  waves: [
    tailEvent(1, { type: "item.started", item: WAVE_LIVE_REASONING }),
    tailEvent(2, { type: "item.started", item: WAVE_LIVE_TOOL }),
  ],
};

export const VISUAL_SESSION_ID = SESSION_ID;
export const VISUAL_ROOT_RUN_ID = ROOT_RUN_ID;
