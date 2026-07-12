import { useCallback } from "react";
import { useInterruptResume } from "./useInterruptResume";

// Submits the user's answers to a clarifying question (API.md §6, R-model) over
// the shared useInterruptResume scaffold. The view collects each
// QuestionField.name as a string (single-select / free-text) or string[]
// (multi); the wire AnswerResponse.answers is always Record<string, string[]>
// (S8), so single values normalize to single-element arrays here at the boundary.

export type QuestionAnswers = Record<string, string | string[]>;

export interface QuestionAnswerSubmit {
  submit: (answers: QuestionAnswers) => void;
  pending: boolean;
}

export function useQuestionAnswer(runId?: string, itemId?: string): QuestionAnswerSubmit {
  const { pending, resume } = useInterruptResume<true>(runId, itemId);

  const submit = useCallback(
    (answers: QuestionAnswers) => {
      const wireAnswers: Record<string, string[]> = {};
      for (const [name, value] of Object.entries(answers)) {
        wireAnswers[name] = Array.isArray(value) ? value : [value];
      }
      // Stamp wireAnswers onto the settled block too, so the collapsed card can
      // echo what was chosen (the runtime never sends the answer back).
      resume(
        true,
        { type: "answer", answers: wireAnswers },
        { answered: true, answers: wireAnswers },
      );
    },
    [resume],
  );

  return { submit, pending: pending !== null };
}
