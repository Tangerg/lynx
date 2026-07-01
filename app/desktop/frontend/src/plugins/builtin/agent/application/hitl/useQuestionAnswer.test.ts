// useQuestionAnswer answers a HITL question interrupt by starting a
// continuation Run via the owning session's `resume` action (API.md §6).
// Worth locking: the InterruptResponse payload (type:"answer" + answers
// normalized to Record<string, string[]>, §6.1 S8), the single-submit guard,
// the pending latch, and the deferred/rolled-back store settle.

import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { useAgentStore } from "@/state/agentStore";
import { useAgentSessionStore } from "@/state/agentSessionStore";
import { useQuestionAnswer } from "./useQuestionAnswer";

const SID = "ses_1";

// resetSession seeds the slice before setResume — the store no longer
// resurrects an absent session, so the binding must follow a reset (as
// useAgentSession does at mount).
function bindResume(impl?: (...args: unknown[]) => void) {
  const resume = impl ? vi.fn(impl) : vi.fn();
  useAgentSessionStore.setState({ activeSessionId: SID });
  useAgentStore.getState().resetSession(SID);
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
    expect(resume).toHaveBeenCalledWith(
      "run_1",
      [
        {
          itemId: "item_q",
          response: { type: "answer", answers: { q1: ["Postgres"], q2: ["tools", "vision"] } },
        },
      ],
      expect.any(Function),
      expect.any(Function),
    );
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
    expect(resume).toHaveBeenCalledWith(
      "run_1",
      [{ itemId: "item_q2", response: { type: "answer", answers: { q1: ["first"] } } }],
      expect.any(Function),
      expect.any(Function),
    );
  });

  it("commits resolveInterrupt only after the run starts; rolls back on reject", () => {
    const onStarted = bindResume((_r, _resp, onSettled) => (onSettled as () => void)());
    const spy = vi.spyOn(useAgentStore.getState(), "resolveInterrupt");
    const { result } = renderHook(() => useQuestionAnswer("run_1", "q_ok"));
    act(() => result.current.submit({ q1: "x" }));
    // The settle patch also stamps the answers so the collapsed card can echo them.
    expect(spy).toHaveBeenCalledWith(SID, "q_ok", { answered: true, answers: { q1: ["x"] } });
    expect(onStarted).toHaveBeenCalled();

    spy.mockClear();
    bindResume((_r, _resp, _s, onStartError) => (onStartError as () => void)());
    const { result: r2 } = renderHook(() => useQuestionAnswer("run_1", "q_fail"));
    act(() => r2.current.submit({ q1: "x" }));
    expect(spy).not.toHaveBeenCalled();
    expect(r2.current.pending).toBe(false); // retryable
    spy.mockRestore();
  });
});
