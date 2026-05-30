// useQuestionAnswer drives the JSON-RPC `runs.question.answer` method. The
// thing worth locking is the wire payload ({ requestId, answers }) and the
// single-submit guard + pending latch (mirrors useApprovalSubmit).

import { act, renderHook } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { resetContainer, setContainer } from "@/main/container";
import type { Methods } from "@/rpc";
import { useQuestionAnswer } from "./useQuestionAnswer";

afterEach(resetContainer);

function stubAnswer() {
  const answer = vi.fn().mockResolvedValue(undefined);
  setContainer({ methods: () => ({ runs: { question: { answer } } }) as unknown as Methods });
  return answer;
}

describe("useQuestionAnswer", () => {
  it("posts { requestId, answers } and latches pending", () => {
    const answer = stubAnswer();
    const { result } = renderHook(() => useQuestionAnswer("q-req-1"));
    const answers = { q1: "Postgres", q2: ["tools", "vision"] };
    act(() => result.current.submit(answers));
    expect(answer).toHaveBeenCalledWith({ requestId: "q-req-1", answers });
    expect(result.current.pending).toBe(true);
  });

  it("no-ops without a requestId, and never double-submits", () => {
    const answer = stubAnswer();
    const { result } = renderHook(() => useQuestionAnswer(undefined));
    act(() => result.current.submit({ q1: "x" }));
    expect(answer).not.toHaveBeenCalled();

    const { result: r2 } = renderHook(() => useQuestionAnswer("q-req-2"));
    const answer2 = stubAnswer();
    act(() => r2.current.submit({ q1: "first" }));
    act(() => r2.current.submit({ q1: "second" })); // ignored — already pending
    expect(answer2).toHaveBeenCalledTimes(1);
    expect(answer2).toHaveBeenCalledWith({ requestId: "q-req-2", answers: { q1: "first" } });
  });
});
