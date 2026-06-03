import { useCallback, useState } from "react";
import { asItemId, asRunId } from "@/rpc";
import { useAgentStore } from "@/state/agentStore";
import { useSessionStore } from "@/state/sessionStore";

// Submits the user's answers to a clarifying question (API.md §6, R-model):
// it answers an open interrupt by starting a continuation Run via the active
// session's `resume` action, and optimistically settles the card via
// `resolveInterrupt`. Addressed by `parentRunId` (the interrupted Run) +
// `itemId` (the question Item). `answers` maps each QuestionField.name to the
// chosen option label(s) (string for single-select, string[] for multi); for
// free-text fields the value is the typed text itself (API.md §6.1).

export type QuestionAnswers = Record<string, string | string[]>;

export interface QuestionAnswerSubmit {
  submit: (answers: QuestionAnswers) => void;
  pending: boolean;
}

export function useQuestionAnswer(parentRunId?: string, itemId?: string): QuestionAnswerSubmit {
  const [pending, setPending] = useState(false);

  const submit = useCallback(
    (answers: QuestionAnswers) => {
      if (!parentRunId || !itemId || pending) return;
      setPending(true);
      const sid = useSessionStore.getState().activeSessionId;
      useAgentStore.getState().resolveInterrupt(sid, itemId, { answered: true });
      const resume = useAgentStore.getState().sessions[sid]?.resume;
      resume?.(asRunId(parentRunId), [
        { itemId: asItemId(itemId), response: { kind: "answer", answers } },
      ]);
    },
    [parentRunId, itemId, pending],
  );

  return { submit, pending };
}
