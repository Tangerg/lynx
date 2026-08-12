// Reducer convergence invariant — the architectural guarantee that streaming,
// history replay, and mixed delivery all render the SAME turn.
//
// There are NOT three rendering code paths. There is ONE fold (`reduce`), fed
// different SUBSETS of the same event stream:
//   - streaming      = the full lifecycle (run.* / item.started / item.delta / item.completed)
//   - history replay = the item.completed-only subset (items.list emits no run/started/delta)
//   - mixed          = any interleaving (some items fully streamed, some snapshot-only —
//                      e.g. hydrated history followed by a live continuation)
// This test pins that convergence so no future change can reintroduce a
// streaming-only quirk (the segment.started turn-reset this guards against was one).
//
// SCOPE — the symmetric payload, whose information lives identically on the
// completed snapshot and the delta stream, so any subset converges:
//   - text / reasoning CONTENT streams via item.delta, and the completed
//     snapshot equals the concatenated deltas;
//   - a call-and-result tool (the generic `tool` below) carries its args as the
//     fully-parsed object on the Item AND, redundantly, as one whole
//     toolArguments delta (the live-preview channel). Args are AUTHORITATIVE
//     from the structured object at the terminal state, so the redundant delta
//     can't make streaming diverge from replay — see writeToolCall.
// One payload is protocol-asymmetric BY DESIGN and is therefore NOT in the
// fixture: commandExecution stdout rides a single item.delta{toolOutput} at
// tool-end and is NOT carried on the completed Item (lynx translator.go /
// docs/protocol/API.md §4.4), so history replay genuinely cannot reconstruct it — an
// information gap in the wire, not divergent fold logic. Exercising the
// symmetric core keeps this invariant an apples-to-apples comparison.

import { beforeEach, describe, expect, it } from "vitest";
import type { AgentItem as Item, AgentStreamEvent as StreamEvent } from "@/plugins/sdk";
import type { AgentSessionView, Message } from "@/plugins/sdk/types/agentSessionView";
import { loadPlugin } from "@/plugins/sdk/definePlugin";
import { foldTestEvent as reduce } from "./reducer.fixtures";
import { appendToTurn } from "./fold";
import { itemStartedAt } from "./projections";
import { EMPTY_AGENT_SESSION_VIEW } from "@/plugins/sdk/types/agentSessionView";

beforeEach(async () => {
  const { default: spec } = await import("@/plugins/builtin/agent/bootstrap/foldPlugin");
  await loadPlugin(spec);
});

function item(partial: Record<string, unknown>): Item {
  return {
    runId: "run_1",
    status: "completed",
    createdAt: "2026-06-03T00:00:00Z",
    ...partial,
  } as Item;
}
const started = (i: Item): StreamEvent => ({
  type: "item.started",
  item: { ...i, status: "running" },
});
const completed = (i: Item): StreamEvent => ({ type: "item.completed", item: i });
const delta = (itemId: string, d: Record<string, unknown>): StreamEvent =>
  ({ type: "item.delta", itemId, delta: d }) as StreamEvent;

const foldAll = (events: StreamEvent[]): AgentSessionView =>
  events.reduce((state, ev) => reduce(state, ev), EMPTY_AGENT_SESSION_VIEW);

// An assistant turn's `createdAt` is wall-clock-stamped when the turn opens (not
// event data), so compare renders without it.
const strip = (msgs: Message[]) => msgs.map(({ id, role, blocks }) => ({ id, role, blocks }));

// id of the Item an item.* event concerns (for the "snapshot this item" filter).
function itemIdOf(e: StreamEvent): string | null {
  if (e.type === "item.started" || e.type === "item.completed") return e.item.id;
  if (e.type === "item.delta") return e.itemId;
  return null;
}

// Drop the started/delta events for the given item ids — i.e. deliver those
// items as completed-only snapshots while the rest stay fully streamed.
function snapshotOnly(events: StreamEvent[], ids: Set<string>): StreamEvent[] {
  return events.filter((e) => {
    const id = itemIdOf(e);
    if (id !== null && ids.has(id) && (e.type === "item.started" || e.type === "item.delta")) {
      return false;
    }
    return true;
  });
}

// One believable turn: user prompt → reasoning → message → tool → message,
// expressed as a FULL streaming sequence. text/reasoning stream via deltas that
// concatenate to the completed snapshot; the tool is call-and-result — its args
// arrive as the parsed object AND a redundant whole toolArguments delta (as the
// real backend sends), and its result whole on completion.
const u1 = item({ id: "u1", type: "userMessage", content: [{ type: "text", text: "delete it" }] });
const r1 = item({ id: "r1", type: "reasoning", text: "Weighing the risk carefully." });
const m1 = item({
  id: "m1",
  type: "agentMessage",
  content: [{ type: "text", text: "Removing the file." }],
});
const t1 = item({
  id: "t1",
  type: "toolCall",
  tool: { name: "fs.delete", arguments: { path: "x" }, result: "ok" },
});
const m2 = item({ id: "m2", type: "agentMessage", content: [{ type: "text", text: "Done." }] });

const FULL_STREAM: StreamEvent[] = [
  { type: "segment.started", run: { id: "run_1", sessionId: "ses_1" } as never },
  started(u1), // user bubble (boundary) — backend sends started(inProgress)+completed
  completed(u1),
  started(r1),
  delta("r1", { type: "reasoning", text: "Weighing the risk " }),
  delta("r1", { type: "reasoning", text: "carefully." }),
  completed(r1),
  started(m1),
  delta("m1", { type: "content", text: "Removing " }),
  delta("m1", { type: "content", text: "the file." }),
  completed(m1),
  started(t1),
  delta("t1", { type: "toolArguments", argumentsTextDelta: '{"path":"x"}' }),
  completed(t1),
  started(m2),
  delta("m2", { type: "content", text: "Done." }),
  completed(m2),
  {
    type: "segment.finished",
    outcome: { type: "completed" },
    metrics: {
      steps: 1,
      activeDurationMillis: 0,
      usage: { inputTokens: 0, outputTokens: 0, cacheReadTokens: 0 },
    },
  },
];

describe("reducer — render convergence across delivery modes", () => {
  it("streaming, replay, and mixed delivery all fold to the same turn", () => {
    const streaming = foldAll(FULL_STREAM);

    // History replay: the item.completed-only subset (no run.* / started / delta).
    const replay = foldAll(FULL_STREAM.filter((e) => e.type === "item.completed"));

    // Mixed: m1 + t1 arrive as completed snapshots, the rest stream live.
    const mixed = foldAll(snapshotOnly(FULL_STREAM, new Set(["m1", "t1"])));

    // Same bubbles, same blocks, same order, same content.
    expect(strip(replay.messages)).toEqual(strip(streaming.messages));
    expect(strip(mixed.messages)).toEqual(strip(streaming.messages));

    // Same tool-call projections (the blocks reference these by id).
    expect(replay.toolCalls).toEqual(streaming.toolCalls);
    expect(mixed.toolCalls).toEqual(streaming.toolCalls);

    // The synthesized turn is dated by the Item that opened it — the runtime's
    // clock, the same one every message beside it carries. The fold reads no
    // clock of its own.
    expect(streaming.messages[1]!.createdAt).toBe(itemStartedAt(r1));

    // Sanity: the fold actually built the turn we described — one user bubble +
    // one assistant turn holding reasoning / text / tool / text, in order.
    expect(streaming.messages).toHaveLength(2);
    expect(streaming.messages[0]!.role).toBe("user");
    expect(streaming.messages[1]!.blocks.map((b) => b.kind)).toEqual([
      "reasoning",
      "text",
      "tool",
      "text",
    ]);
  });
});

describe("fold — one message per id", () => {
  it("re-adopts an item's turn instead of minting its id twice", () => {
    // A user message (a send, or a mid-run steer) closes the open turn. If a
    // later block for the SAME item comes back, the fold must land in the turn it
    // already minted for that item — two messages under one React key is the
    // duplicate-key loop CLAUDE.md §5 names.
    const first = appendToTurn(EMPTY_AGENT_SESSION_VIEW, "run_1", "m9", {
      kind: "text",
      text: "a",
      status: "running",
    });
    const closed: AgentSessionView = { ...first, assistantTurnByRunId: {} };
    const second = appendToTurn(closed, "run_1", "m9", {
      kind: "reasoning",
      reasoningId: "m9",
      text: "b",
      status: "running",
    });

    const ids = second.messages.map((m) => m.id);
    expect(ids).toEqual(["turn:m9"]);
    expect(second.assistantTurnByRunId.run_1).toBe("turn:m9");
    expect(second.messages[0]!.blocks.map((b) => b.kind)).toEqual(["text", "reasoning"]);
  });
});
