import { describe, expect, it } from "vitest";
import type { QuestionItem } from "@/plugins/builtin/agent/public/viewState";
import {
  createQuestionDraft,
  setQuestionText,
} from "@/plugins/builtin/agent/public/messagePresentation";
import { canSubmitQuestionCard, questionCardSettledView } from "./questionCardModel";

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

  it("echoes the local draft while a submit is pending", () => {
    const draft = setQuestionText(createQuestionDraft([question]), 0, question, "Extract model");

    expect(
      questionCardSettledView({
        status: "requires-action",
        pending: true,
        questions: [question],
        draft,
      }),
    ).toEqual({ settled: true, answers: [["Extract model"]] });
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
});

describe("canSubmitQuestionCard", () => {
  it("requires a resumable complete non-pending question", () => {
    expect(
      canSubmitQuestionCard({
        runId: "run",
        itemId: "item",
        status: "requires-action",
        complete: true,
        pending: false,
      }),
    ).toBe(true);
    expect(
      canSubmitQuestionCard({
        runId: "run",
        itemId: "item",
        status: "requires-action",
        complete: true,
        pending: true,
      }),
    ).toBe(false);
    expect(
      canSubmitQuestionCard({
        runId: "run",
        itemId: "item",
        status: "requires-action",
        complete: false,
        pending: false,
      }),
    ).toBe(false);
  });
});
