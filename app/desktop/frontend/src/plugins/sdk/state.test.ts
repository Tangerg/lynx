import type { AgentViewState, Message } from "@/plugins/sdk/types/agentView";
import { describe, expect, it } from "vitest";
import { INITIAL_VIEW_STATE } from "@/plugins/sdk/types/agentView";
import {
  appendBlockToLatestAssistant,
  appendBlockToMessage,
  compose,
  patchRun,
  setPlan,
} from "./state";

// Helpers to construct messages without typing the whole shape every time.
const msg = (id: string, role: Message["role"] = "assistant"): Message => ({
  id,
  role,
  who: role,
  time: "0:00",
  blocks: [],
});

const stateWith = (messages: Message[]): AgentViewState => ({
  ...INITIAL_VIEW_STATE,
  messages,
});

describe("appendBlockToMessage", () => {
  it("appends to the matching message id", () => {
    const update = appendBlockToMessage("m1", { kind: "text", text: "hi", status: "complete" });
    const next = update(stateWith([msg("m1"), msg("m2")]));

    expect(next.messages[0]!.blocks).toHaveLength(1);
    expect(next.messages[1]!.blocks).toHaveLength(0);
  });

  it("is a no-op when the id is missing", () => {
    const initial = stateWith([msg("m1")]);
    const update = appendBlockToMessage("nope", { kind: "text", text: "x", status: "complete" });
    const next = update(initial);
    expect(next.messages[0]!.blocks).toHaveLength(0);
  });
});

describe("appendBlockToLatestAssistant", () => {
  it("targets the most-recent assistant message", () => {
    const update = appendBlockToLatestAssistant({ kind: "plan" });
    const next = update(
      stateWith([
        msg("u1", "user"),
        msg("a1"), // assistant — not latest
        msg("u2", "user"),
        msg("a2"), // latest assistant
      ]),
    );

    expect(next.messages[1]!.blocks).toHaveLength(0);
    expect(next.messages[3]!.blocks).toHaveLength(1);
    expect(next.messages[3]!.blocks[0]).toEqual({ kind: "plan" });
  });

  it("is a no-op when no assistant messages exist", () => {
    const update = appendBlockToLatestAssistant({ kind: "plan" });
    const initial = stateWith([msg("u1", "user")]);
    expect(update(initial)).toBe(initial);
  });
});

describe("setPlan", () => {
  it("replaces the plan array wholesale", () => {
    const update = setPlan([{ id: 1, pid: "T-1", status: "doing", text: "x" }]);
    const next = update(stateWith([]));
    expect(next.plan).toEqual([{ id: 1, pid: "T-1", status: "doing", text: "x" }]);
  });
});

describe("patchRun", () => {
  it("merges into run state", () => {
    const update = patchRun({ activity: "scanning", step: 42 });
    const next = update(INITIAL_VIEW_STATE);
    expect(next.run.activity).toBe("scanning");
    expect(next.run.step).toBe(42);
    // Untouched fields keep their values.
    expect(next.run.usage).toBe(INITIAL_VIEW_STATE.run.usage);
  });
});

describe("compose", () => {
  it("applies updates left-to-right", () => {
    const update = compose(
      setPlan([{ id: 1, pid: "T-1", status: "todo", text: "a" }]),
      patchRun({ activity: "building" }),
    );
    const next = update(INITIAL_VIEW_STATE);
    expect(next.plan).toHaveLength(1);
    expect(next.run.activity).toBe("building");
  });

  it("returns the original state when called with zero updates", () => {
    const update = compose();
    expect(update(INITIAL_VIEW_STATE)).toBe(INITIAL_VIEW_STATE);
  });
});
