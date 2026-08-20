import { describe, expect, it } from "vitest";
import type { QuestionItem } from "@/plugins/builtin/agent/public/viewState";
import type { TranscriptRow } from "@/plugins/builtin/agent/public/conversation";
import {
  createQuestionDraft,
  setQuestionText,
} from "@/plugins/builtin/agent/public/messagePresentation";
import {
  canSubmitQuestionCard,
  pendingQuestionRequest,
  questionCardSettledView,
} from "./questionCardModel";

const question: QuestionItem = {
  type: "text",
  header: "Goal",
  prompt: "What should change?",
};

describe("questionCardSettledView", () => {
  it("uses stamped answers for completed questions", () => {
    const draft = createQuestionDraft([question]);

    expect(
      questionCardSettledView({
        status: "complete",
        answered: true,
        pending: false,
        questions: [question],
        draft,
        answers: [["Refactor"]],
      }),
    ).toEqual({ settled: true, answers: [["Refactor"]] });
  });

  it("keeps the request surface until the Runtime settles a pending submit", () => {
    const draft = setQuestionText(createQuestionDraft([question]), 0, question, "Extract model");

    expect(
      questionCardSettledView({
        status: "requires-action",
        pending: true,
        questions: [question],
        draft,
      }),
    ).toEqual({ settled: false });
  });

  it("stays interactive before a question is answered", () => {
    expect(
      questionCardSettledView({
        status: "requires-action",
        pending: false,
        questions: [question],
        draft: createQuestionDraft([question]),
      }),
    ).toEqual({ settled: false });
  });

  it("never promotes a losing local draft after the Runtime settles another answer", () => {
    const draft = setQuestionText(createQuestionDraft([question]), 0, question, "local loser");

    expect(
      questionCardSettledView({
        status: "complete",
        answered: true,
        pending: false,
        questions: [question],
        draft,
        answers: [["authoritative winner"]],
      }),
    ).toEqual({ settled: true, answers: [["authoritative winner"]] });
  });

  it("shows no answer when a question closes without an accepted response", () => {
    const draft = setQuestionText(createQuestionDraft([question]), 0, question, "never accepted");

    expect(
      questionCardSettledView({
        status: "complete",
        answered: false,
        pending: false,
        questions: [question],
        draft,
      }),
    ).toEqual({ settled: true, answers: undefined });
  });
});

describe("canSubmitQuestionCard", () => {
  it("requires a resumable non-pending question and permits explicit skip", () => {
    expect(
      canSubmitQuestionCard({
        runId: "run",
        itemId: "item",
        status: "requires-action",
        pending: false,
      }),
    ).toBe(true);
    expect(
      canSubmitQuestionCard({
        runId: "run",
        itemId: "item",
        status: "requires-action",
        pending: true,
      }),
    ).toBe(false);
    expect(
      canSubmitQuestionCard({
        runId: undefined,
        itemId: "item",
        status: "requires-action",
        pending: false,
      }),
    ).toBe(false);
  });
});

describe("pendingQuestionRequest", () => {
  it("selects the latest unanswered question without inventing another read model", () => {
    const block = {
      kind: "question" as const,
      status: "requires-action" as const,
      runId: "run",
      itemId: "item",
      questions: [question],
    };
    const row = {
      message: { id: "message", role: "assistant", runId: "run", blocks: [block] },
      runOwner: { kind: "owned", runId: "run", status: "waiting" },
      facts: { toolCalls: {}, delegatedRuns: {} },
    } as TranscriptRow;

    expect(pendingQuestionRequest([row])).toBe(block);
    expect(
      pendingQuestionRequest([
        {
          ...row,
          message: {
            ...row.message,
            blocks: [{ ...block, status: "complete", answered: true }],
          },
        },
      ]),
    ).toBeNull();
  });
});
