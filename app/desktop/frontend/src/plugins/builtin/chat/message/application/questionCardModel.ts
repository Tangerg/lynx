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
  if (!questionSettled(status, answered) && !pending) return { settled: false };
  return { settled: true, answers: questionSettledAnswers(questions, draft, answers) };
}

export function canSubmitQuestionCard({
  parentRunId,
  itemId,
  status,
  complete,
  pending,
}: {
  parentRunId?: string;
  itemId?: string;
  status: BlockStatus;
  complete: boolean;
  pending: boolean;
}): boolean {
  return !pending && canSubmitQuestion({ parentRunId, itemId, complete, status });
}

export function useQuestionCardActions({
  parentRunId,
  itemId,
  status,
  questions,
  draft,
}: {
  parentRunId?: string;
  itemId?: string;
  status: BlockStatus;
  questions: readonly QuestionItem[];
  draft: QuestionDraft;
}) {
  const { submit, pending } = useQuestionAnswer(parentRunId, itemId);
  const complete = useMemo(() => questionDraftComplete(questions, draft), [questions, draft]);
  const payload = useMemo(() => questionDraftAnswers(questions, draft), [questions, draft]);

  const submitAnswer = useCallback(() => {
    submit(payload);
  }, [payload, submit]);

  return {
    pending,
    disabled: !canSubmitQuestionCard({ parentRunId, itemId, status, complete, pending }),
    submit: submitAnswer,
  };
}
