import { describe, expect, it } from "vitest";
import type { QuestionItem } from "@/plugins/sdk/types/contentBlock";
import {
  canSubmitQuestion,
  createQuestionDraft,
  questionAnswerText,
  questionDraftAnswers,
  questionDraftComplete,
  questionSettled,
  questionSettledAnswers,
  setQuestionText,
  toggleQuestionOption,
} from "./questionPresentation";

const single: QuestionItem = {
  type: "choice",
  header: "Choice",
  prompt: "Pick one",
  options: [
    { label: "A", description: "Alpha" },
    { label: "B", description: "Beta" },
  ],
  multiple: false,
  allowCustom: true,
};

const multi: QuestionItem = {
  type: "choice",
  header: "Multi",
  prompt: "Pick many",
  options: [
    { label: "A", description: "Alpha" },
    { label: "B", description: "Beta" },
  ],
  multiple: true,
  allowCustom: true,
};

const closed: QuestionItem = {
  ...single,
  allowCustom: false,
};

describe("questionPresentation", () => {
  it("creates an empty draft for every question", () => {
    expect(createQuestionDraft([single, multi])).toEqual([
      { selected: [], text: "" },
      { selected: [], text: "" },
    ]);
  });

  it("tracks draft completeness", () => {
    expect(questionDraftComplete([], [])).toBe(false);
    let draft = createQuestionDraft([single]);
    expect(questionDraftComplete([single], draft)).toBe(false);
    draft = toggleQuestionOption(draft, 0, single, "A");
    expect(questionDraftComplete([single], draft)).toBe(true);
  });

  it("keeps single-select option and text mutually exclusive", () => {
    let draft = createQuestionDraft([single]);
    draft = toggleQuestionOption(draft, 0, single, "A");
    expect(draft[0]).toEqual({ selected: ["A"], text: "" });
    draft = setQuestionText(draft, 0, single, "custom");
    expect(draft[0]).toEqual({ selected: [], text: "custom" });
  });

  it("unions multi-select options and free text", () => {
    let draft = createQuestionDraft([multi]);
    draft = toggleQuestionOption(draft, 0, multi, "A");
    draft = toggleQuestionOption(draft, 0, multi, "B");
    draft = setQuestionText(draft, 0, multi, "other");
    expect(questionDraftAnswers([multi], draft)).toEqual([["A", "B", "other"]]);
  });

  it("does not duplicate a selected option entered as custom text", () => {
    let draft = createQuestionDraft([multi]);
    draft = toggleQuestionOption(draft, 0, multi, "A");
    draft = setQuestionText(draft, 0, multi, "A");
    expect(questionDraftAnswers([multi], draft)).toEqual([["A"]]);
  });

  it("does not manufacture custom answers for a closed choice", () => {
    const draft = createQuestionDraft([closed]);
    expect(setQuestionText(draft, 0, closed, "other")).toBe(draft);
    expect(questionDraftComplete([closed], draft)).toBe(false);
    expect(questionDraftAnswers([closed], draft)).toEqual([[""]]);
  });

  it("formats answer echoes", () => {
    const answers = [["A"], ["A", "B"]];
    expect(questionAnswerText(answers, 0)).toBe("A");
    expect(questionAnswerText(answers, 1)).toBe("A, B");
    expect(questionAnswerText(answers, 2)).toBe("");
  });

  it("derives settled state from block status or optimistic answer stamp", () => {
    expect(questionSettled("complete", false)).toBe(true);
    expect(questionSettled("requires-action", true)).toBe(true);
    expect(questionSettled("requires-action", false)).toBe(false);
  });

  it("uses stamped answers before falling back to a complete local draft", () => {
    let draft = createQuestionDraft([single]);
    expect(questionSettledAnswers([single], draft, undefined)).toBeUndefined();

    draft = toggleQuestionOption(draft, 0, single, "A");
    expect(questionSettledAnswers([single], draft, [["B"]])).toEqual([["B"]]);
    expect(questionSettledAnswers([single], draft, undefined)).toEqual([["A"]]);
  });

  it("submits only open resumable questions with complete answers", () => {
    expect(
      canSubmitQuestion({
        runId: "run_1",
        itemId: "item_1",
        complete: true,
        status: "requires-action",
      }),
    ).toBe(true);
    expect(
      canSubmitQuestion({
        runId: "run_1",
        itemId: "item_1",
        complete: false,
        status: "requires-action",
      }),
    ).toBe(false);
    expect(
      canSubmitQuestion({
        runId: "run_1",
        itemId: "item_1",
        complete: true,
        status: "incomplete",
      }),
    ).toBe(false);
  });
});
