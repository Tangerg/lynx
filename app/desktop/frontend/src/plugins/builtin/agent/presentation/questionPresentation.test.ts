import { describe, expect, it } from "vitest";
import type { QuestionItem } from "@/plugins/sdk/types/agentView";
import {
  createQuestionDraft,
  questionAnswerText,
  questionDraftAnswers,
  questionDraftComplete,
  setQuestionText,
  toggleQuestionOption,
} from "./questionPresentation";

const single: QuestionItem = {
  id: "choice",
  header: "Choice",
  question: "Pick one",
  options: [{ label: "A", description: "Alpha" }],
  multiSelect: false,
  allowFreeText: true,
};

const multi: QuestionItem = {
  id: "multi",
  header: "Multi",
  question: "Pick many",
  options: [
    { label: "A", description: "Alpha" },
    { label: "B", description: "Beta" },
  ],
  multiSelect: true,
  allowFreeText: true,
};

describe("questionPresentation", () => {
  it("creates an empty draft for every question", () => {
    expect(createQuestionDraft([single, multi])).toEqual({
      choice: { selected: [], text: "" },
      multi: { selected: [], text: "" },
    });
  });

  it("tracks draft completeness", () => {
    let draft = createQuestionDraft([single]);
    expect(questionDraftComplete([single], draft)).toBe(false);
    draft = toggleQuestionOption(draft, single, "A");
    expect(questionDraftComplete([single], draft)).toBe(true);
  });

  it("keeps single-select option and text mutually exclusive", () => {
    let draft = createQuestionDraft([single]);
    draft = toggleQuestionOption(draft, single, "A");
    expect(draft.choice).toEqual({ selected: ["A"], text: "" });
    draft = setQuestionText(draft, single, "custom");
    expect(draft.choice).toEqual({ selected: [], text: "custom" });
  });

  it("unions multi-select options and free text", () => {
    let draft = createQuestionDraft([multi]);
    draft = toggleQuestionOption(draft, multi, "A");
    draft = toggleQuestionOption(draft, multi, "B");
    draft = setQuestionText(draft, multi, "other");
    expect(questionDraftAnswers([multi], draft)).toEqual({ multi: ["A", "B", "other"] });
  });

  it("formats answer echoes", () => {
    expect(questionAnswerText({ choice: "A", multi: ["A", "B"] }, "choice")).toBe("A");
    expect(questionAnswerText({ choice: "A", multi: ["A", "B"] }, "multi")).toBe("A, B");
    expect(questionAnswerText({}, "missing")).toBe("");
  });
});
