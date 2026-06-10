import { useCallback, useState } from "react";
import { asItemId, asRunId } from "@/rpc";
import { useAgentStore } from "@/state/agentStore";
import { useSessionStore } from "@/state/sessionStore";

// Submits the user's answers to a clarifying question (API.md §6, R-model):
// it answers an open interrupt by starting a continuation Run via the owning
// session's `resume` action. Like useApprovalSubmit, the store-level settle
// (`resolveInterrupt`) is deferred to the run-started callback so a rejected
// resume stays retryable. Addressed by `parentRunId` (the interrupted Run) +
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
  // Pin the owning session at mount — see useApprovalSubmit for why.
  const [sessionId] = useState(() => useSessionStore.getState().activeSessionId);

  const submit = useCallback(
    (answers: QuestionAnswers) => {
      if (!parentRunId || !itemId || pending) return;
      const resume = useAgentStore.getState().sessions[sessionId]?.resume;
      if (!resume) return;
      setPending(true);
      // Normalize to Record<string, string[]> — single-select / free-text
      // values become single-element arrays (wire AnswerResponse, §6.1 S8).
      const wireAnswers: Record<string, string[]> = {};
      for (const [name, value] of Object.entries(answers)) {
        wireAnswers[name] = Array.isArray(value) ? value : [value];
      }
      // resolveInterrupt is deferred to the success callback so a channel-a
      // failure leaves the question card retryable (see useApprovalSubmit).
      resume(
        asRunId(parentRunId),
        [{ itemId: asItemId(itemId), response: { type: "answer", answers: wireAnswers } }],
        () => useAgentStore.getState().resolveInterrupt(sessionId, itemId, { answered: true }),
        () => setPending(false),
      );
    },
    [parentRunId, itemId, pending, sessionId],
  );

  return { submit, pending };
}
