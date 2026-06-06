import { useCallback, useState } from "react";
import { asItemId, asRunId } from "@/rpc";
import { useAgentStore } from "@/state/agentStore";
import { useSessionStore } from "@/state/sessionStore";

// Submits the user's answers to a clarifying question (API.md §6, R-model):
// it answers an open interrupt by starting a continuation Run via the active
// session's `resume` action, and optimistically settles the card via
// `resolveInterrupt`. Addressed by `parentRunId` (the interrupted Run) +
// `itemId` (the question Item). The view collects each QuestionField.name as a
// string (single-select / free-text) or string[] (multi); the wire
// AnswerResponse.answers is always Record<string, string[]> (S8), so we
// normalize single values to single-element arrays here at the boundary.

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
      // Normalize to Record<string, string[]> — single-select / free-text
      // values become single-element arrays (wire AnswerResponse, §6.1 S8).
      const wireAnswers: Record<string, string[]> = {};
      for (const [name, value] of Object.entries(answers)) {
        wireAnswers[name] = Array.isArray(value) ? value : [value];
      }
      resume?.(asRunId(parentRunId), [
        { itemId: asItemId(itemId), response: { type: "answer", answers: wireAnswers } },
      ]);
    },
    [parentRunId, itemId, pending],
  );

  return { submit, pending };
}
