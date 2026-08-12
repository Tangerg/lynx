import { useCallback, useMemo } from "react";
import type { BlockStatus, QuestionItem } from "@/plugins/builtin/agent/public/viewState";
import { useQuestionAnswer } from "@/plugins/builtin/agent/public/hitl";
import {
  canSubmitQuestion,
  questionDraftAnswers,
  questionDraftComplete,
  questionSettled,
  questionSettledAnswers,
  type QuestionAnswers,
  type QuestionDraft,
} from "@/plugins/builtin/agent/public/messagePresentation";

export interface QuestionCardSettledView {
  settled: boolean;
  answers?: QuestionAnswers;
}

export function questionCardSettledView({
  status,
  answered,
  pending,
  questions,
  draft,
  answers,
}: {
  status: BlockStatus;
  answered?: boolean;
  pending: boolean;
  questions: readonly QuestionItem[];
  draft: QuestionDraft;
  answers?: QuestionAnswers;
}): QuestionCardSettledView {
  if (pending) {
    return { settled: true, answers: questionSettledAnswers(questions, draft, answers) };
  }
  if (!questionSettled(status, answered)) return { settled: false };
  // Once the Runtime closes the Pending set, only its transcript projection is
  // authoritative. A local draft may have lost a cross-client race or the Run
  // may have been canceled without accepting any answer.
  return { settled: true, answers };
}

export function canSubmitQuestionCard({
  runId,
  itemId,
  status,
  complete,
  pending,
}: {
  runId?: string;
  itemId?: string;
  status: BlockStatus;
  complete: boolean;
  pending: boolean;
}): boolean {
  return !pending && canSubmitQuestion({ runId, itemId, complete, status });
}

export function useQuestionCardActions({
  runId,
  itemId,
  status,
  questions,
  draft,
}: {
  runId?: string;
  itemId?: string;
  status: BlockStatus;
  questions: readonly QuestionItem[];
  draft: QuestionDraft;
}) {
  const { submit, pending } = useQuestionAnswer(runId, itemId);
  const complete = useMemo(() => questionDraftComplete(questions, draft), [questions, draft]);
  const payload = useMemo(() => questionDraftAnswers(questions, draft), [questions, draft]);

  const submitAnswer = useCallback(() => {
    submit(payload);
  }, [payload, submit]);

  return {
    pending,
    disabled: !canSubmitQuestionCard({ runId, itemId, status, complete, pending }),
    submit: submitAnswer,
  };
}
