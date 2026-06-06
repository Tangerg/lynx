// useQuestionAnswer answers a HITL question interrupt by starting a
// continuation Run via the active session's `resume` action (API.md §6).
// Worth locking: the InterruptResponse payload (type:"answer" + answers
// normalized to Record<string, string[]>, §6.1 S8), the single-submit guard,
// and the pending latch.

import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useAgentStore } from "@/state/agentStore";
import { useSessionStore } from "@/state/sessionStore";
import { useQuestionAnswer } from "./useQuestionAnswer";

const SID = "ses_1";

function bindResume() {
  const resume = vi.fn();
  useSessionStore.setState({ activeSessionId: SID });
  useAgentStore.getState().setResume(SID, resume);
  return resume;
}

afterEach(() => useAgentStore.getState().dropSession(SID));
beforeEach(() => useAgentStore.getState().dropSession(SID));

describe("useQuestionAnswer", () => {
  it("resumes with an answer InterruptResponse and latches pending", () => {
    const resume = bindResume();
    const { result } = renderHook(() => useQuestionAnswer("run_1", "item_q"));
    const answers = { q1: "Postgres", q2: ["tools", "vision"] };
    act(() => result.current.submit(answers));
    // Single-select / free-text values are normalized to single-element arrays
    // (wire AnswerResponse.answers is always Record<string, string[]>, §6.1 S8).
    expect(resume).toHaveBeenCalledWith("run_1", [
      {
        itemId: "item_q",
        response: { type: "answer", answers: { q1: ["Postgres"], q2: ["tools", "vision"] } },
      },
    ]);
    expect(result.current.pending).toBe(true);
  });

  it("no-ops without a parentRunId/itemId, and never double-submits", () => {
    const resume = bindResume();
    const { result } = renderHook(() => useQuestionAnswer(undefined, undefined));
    act(() => result.current.submit({ q1: "x" }));
    expect(resume).not.toHaveBeenCalled();

    const { result: r2 } = renderHook(() => useQuestionAnswer("run_1", "item_q2"));
    act(() => r2.current.submit({ q1: "first" }));
    act(() => r2.current.submit({ q1: "second" })); // ignored — already pending
    expect(resume).toHaveBeenCalledTimes(1);
    expect(resume).toHaveBeenCalledWith("run_1", [
      { itemId: "item_q2", response: { type: "answer", answers: { q1: ["first"] } } },
    ]);
  });
});
