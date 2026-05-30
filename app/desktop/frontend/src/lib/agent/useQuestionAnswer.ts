import { useCallback, useState } from "react";
import { getContainer } from "@/main/container";

// Submits the user's answers to a clarifying question via the JSON-RPC
// `runs.question.answer` method (the cached client from
// `container.methods()`). Parallels useApprovalSubmit — only the wire
// payload differs (a per-question answer map keyed by question.id).
//
// `answers` maps each question's stable `id` to the chosen option label(s)
// (string for single-select, string[] for multi-select). For free-text
// questions the value is the typed text itself, not an "Other" marker
// (API.md §4.4).
//
// We deliberately keep `pending` set on success: the backend's
// `lyra.question-result` event flips the block to `answered`, and the card
// renders against that. Clearing here would flicker the card back.

export type QuestionAnswers = Record<string, string | string[]>;

export interface QuestionAnswerSubmit {
  submit: (answers: QuestionAnswers) => void;
  pending: boolean;
}

export function useQuestionAnswer(requestId: string | undefined): QuestionAnswerSubmit {
  const [pending, setPending] = useState(false);

  const submit = useCallback(
    (answers: QuestionAnswers) => {
      if (!requestId || pending) return;
      setPending(true);
      getContainer()
        .methods()
        .runs.question.answer({ requestId, answers })
        .catch((err: unknown) => {
          console.error("[question] answer rejected:", err);
          setPending(false);
        });
    },
    [requestId, pending],
  );

  return { submit, pending };
}
