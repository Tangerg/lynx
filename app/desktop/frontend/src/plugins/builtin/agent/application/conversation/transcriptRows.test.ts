import { describe, expect, it } from "vitest";
import {
  EMPTY_AGENT_SESSION_VIEW,
  type AgentRunView,
  type AgentSessionView,
  type Message,
  type ToolCall,
} from "@/plugins/sdk/types/agentSessionView";
import type { ContentBlock } from "@/plugins/sdk/types/contentBlock";
import { buildTranscriptRows, EMPTY_TRANSCRIPT_ROW_CACHE } from "./transcriptRows";

const ROOT_RUN = "root-run";

function run(overrides: Partial<AgentRunView> & Pick<AgentRunView, "id">): AgentRunView {
  return {
    sessionId: "session-1",
    parentRunId: null,
    rootRunId: overrides.id,
    spawnedByItemId: null,
    status: "finished",
    activeSegmentId: null,
    outcome: { type: "completed" },
    metrics: {
      steps: 1,
      activeDurationMillis: 10,
      usage: { inputTokens: 1, outputTokens: 1, cacheReadTokens: 0 },
    },
    progress: null,
    createdAt: "2026-01-01T00:00:00.000Z",
    finishedAt: "2026-01-01T00:00:01.000Z",
    ...overrides,
  };
}

function tool(id: string, args = "{}"): ToolCall {
  return { id, runId: ROOT_RUN, name: "read_file", fn: "Read", args, status: "ok" };
}

function message(id: string, blocks: ContentBlock[], runId: string | null = ROOT_RUN): Message {
  return { id, runId, role: "assistant", blocks };
}

function text(body: string): ContentBlock {
  return { kind: "text", text: body, status: "complete" };
}

function toolBlock(toolCallId: string): ContentBlock {
  return { kind: "tool", toolCallId };
}

function view(options: {
  messages: Message[];
  toolCalls?: ToolCall[];
  runs?: AgentRunView[];
}): AgentSessionView {
  const runs = [run({ id: ROOT_RUN }), ...(options.runs ?? [])];
  return {
    ...EMPTY_AGENT_SESSION_VIEW,
    messages: options.messages,
    runsById: Object.fromEntries(runs.map((item) => [item.id, item])),
    toolCalls: Object.fromEntries((options.toolCalls ?? []).map((item) => [item.id, item])),
  };
}

/**
 * These are performance-contract tests, and identity is the contract: a row React can
 * skip is exactly a row whose object came back unchanged. Asserting on content instead
 * would pass just as happily against the projection that rebuilt every row on every
 * delta, which is the regression this file exists to catch.
 */
describe("transcript rows", () => {
  it("keeps every untouched row identical when the tail streams", () => {
    const first = message("m1", [text("hello")]);
    const second = message("m2", [text("answer")]);
    const before = buildTranscriptRows(
      view({ messages: [first, second] }),
      EMPTY_TRANSCRIPT_ROW_CACHE,
    );

    // What a text delta does: the fold replaces the tail message and leaves the rest at
    // the same reference.
    const grown = message("m2", [text("answer, continued")]);
    const after = buildTranscriptRows(view({ messages: [first, grown] }), before.cache);

    expect(after.rows[0]).toBe(before.rows[0]);
    expect(after.rows[1]).not.toBe(before.rows[1]);
    expect(after.rows[1]?.message).toBe(grown);
  });

  it("invalidates a row only when its exact Run crosses a lifecycle boundary", () => {
    const turn = message("m1", [text("temporarily complete")]);
    const running = run({
      id: ROOT_RUN,
      status: "running",
      activeSegmentId: "segment-1",
      outcome: null,
      finishedAt: null,
    });
    const before = buildTranscriptRows(
      view({ messages: [turn], runs: [running] }),
      EMPTY_TRANSCRIPT_ROW_CACHE,
    );

    expect(before.rows[0]?.runOwner).toEqual({
      kind: "owned",
      runId: ROOT_RUN,
      status: "running",
    });

    const progressOnly = buildTranscriptRows(
      view({
        messages: [turn],
        runs: [
          {
            ...running,
            progress: { activity: "still streaming" },
          },
        ],
      }),
      before.cache,
    );
    expect(progressOnly.rows[0]).toBe(before.rows[0]);

    const finished = buildTranscriptRows(view({ messages: [turn] }), progressOnly.cache);
    expect(finished.rows[0]).not.toBe(before.rows[0]);
    expect(finished.rows[0]?.runOwner).toEqual({
      kind: "owned",
      runId: ROOT_RUN,
      status: "finished",
    });
  });

  it("keeps an optimistic turn explicitly unassigned until its durable Run arrives", () => {
    const build = buildTranscriptRows(
      view({
        messages: [message("local", [text("draft")], null)],
      }),
      EMPTY_TRANSCRIPT_ROW_CACHE,
    );

    expect(build.rows[0]?.runOwner).toEqual({ kind: "unassigned" });
  });

  it("leaves a turn showing no tool call alone when a tool streams its arguments", () => {
    const prose = message("m1", [text("hello")]);
    const withTool = message("m2", [toolBlock("t1")]);
    const messages = [prose, withTool];
    const before = buildTranscriptRows(
      view({ messages, toolCalls: [tool("t1")] }),
      EMPTY_TRANSCRIPT_ROW_CACHE,
    );

    // Only the tool object changes — every message stays at its own reference, which is
    // exactly what a TOOL_CALL_ARGS delta looks like.
    const after = buildTranscriptRows(
      view({ messages, toolCalls: [tool("t1", '{"path":"a.ts"}')] }),
      before.cache,
    );

    expect(after.rows[0]).toBe(before.rows[0]);
    expect(after.rows[1]).not.toBe(before.rows[1]);
    expect(after.rows[1]?.facts.toolCalls.t1?.args).toBe('{"path":"a.ts"}');
  });

  it("gives a turn only the calls it shows", () => {
    const build = buildTranscriptRows(
      view({
        messages: [message("m1", [toolBlock("t1")]), message("m2", [toolBlock("t2")])],
        toolCalls: [tool("t1"), tool("t2")],
      }),
      EMPTY_TRANSCRIPT_ROW_CACHE,
    );

    expect(Object.keys(build.rows[0]?.facts.toolCalls ?? {})).toEqual(["t1"]);
    expect(Object.keys(build.rows[1]?.facts.toolCalls ?? {})).toEqual(["t2"]);
  });

  it("reaches through delegation transitively, and re-slices when a subagent's turn grows", () => {
    const child = run({
      id: "child-run",
      parentRunId: ROOT_RUN,
      rootRunId: ROOT_RUN,
      spawnedByItemId: "t1",
    });
    const grandchild = run({
      id: "grandchild-run",
      parentRunId: "child-run",
      rootRunId: ROOT_RUN,
      spawnedByItemId: "t2",
    });
    const childTurn = message("child-1", [toolBlock("t2")], "child-run");
    const root = message("m1", [toolBlock("t1")]);
    const options = {
      toolCalls: [tool("t1"), tool("t2")],
      runs: [child, grandchild],
    };

    const before = buildTranscriptRows(
      view({ ...options, messages: [root, childTurn] }),
      EMPTY_TRANSCRIPT_ROW_CACHE,
    );

    // One row: the delegated turn is material UNDER the root turn, not a row beside it.
    expect(before.rows).toHaveLength(1);
    // The grandchild is reachable only via the child's own tool call, so a row that
    // stopped walking at depth one would render the nested subagent as nothing.
    expect(Object.keys(before.rows[0]?.facts.delegatedRuns ?? {}).sort()).toEqual(["t1", "t2"]);

    const grownChildTurn = message("child-1", [toolBlock("t2"), text("done")], "child-run");
    const after = buildTranscriptRows(
      view({ ...options, messages: [root, grownChildTurn] }),
      before.cache,
    );
    expect(after.rows[0]).not.toBe(before.rows[0]);
  });

  it("shares one empty facts object across turns that show nothing", () => {
    const build = buildTranscriptRows(
      view({ messages: [message("m1", [text("a")]), message("m2", [text("b")])] }),
      EMPTY_TRANSCRIPT_ROW_CACHE,
    );

    // Not merely equal — the SAME object. A fresh `{}` per turn would make every
    // text-only row a new row on every delta even with the cache in place.
    expect(build.rows[0]?.facts).toBe(build.rows[1]?.facts);
  });

  it("drops a turn that left the transcript instead of pinning it in the cache", () => {
    const kept = message("m1", [text("a")]);
    const before = buildTranscriptRows(
      view({ messages: [kept, message("m2", [text("b")])] }),
      EMPTY_TRANSCRIPT_ROW_CACHE,
    );
    const after = buildTranscriptRows(view({ messages: [kept] }), before.cache);

    expect(after.rows).toHaveLength(1);
    expect(after.rows[0]).toBe(before.rows[0]);
    expect(after.cache.has("m2")).toBe(false);
  });
});
