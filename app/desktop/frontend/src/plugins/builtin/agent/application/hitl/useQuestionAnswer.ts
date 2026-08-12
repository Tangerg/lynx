import { useCallback } from "react";
import { useInterruptResume } from "./useInterruptResume";

// Submits the user's answers to a clarifying question (API.md §6, R-model) over
// the shared useInterruptResume scaffold, which collects each card into the
// owning root's complete response set. Answers preserve Question.fields order
// and every field always contributes one values array. This is already the wire
// shape, so no dynamic-key normalization or field-name join exists at the
// boundary.

export type QuestionAnswers = string[][];

export interface QuestionAnswerSubmit {
  submit: (answers: QuestionAnswers) => void;
  pending: boolean;
}

export function useQuestionAnswer(runId?: string, itemId?: string): QuestionAnswerSubmit {
  const { pending, resume } = useInterruptResume<true>(runId, itemId);

  const submit = useCallback(
    (answers: QuestionAnswers) => {
      // The local settle removes interaction latency after the Runtime accepted
      // the claim. Durable refresh/replay then replaces it with the same
      // authoritative Question.answers projection.
      resume(true, { type: "answer", answers }, { answered: true, answers });
    },
    [resume],
  );

  return { submit, pending: pending !== null };
}
