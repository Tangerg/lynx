import { useCallback, useMemo } from "react";
import type { BlockStatus, QuestionItem } from "@/plugins/builtin/agent/public/viewState";
import type { TranscriptRow } from "@/plugins/builtin/agent/public/conversation";
import { useQuestionAnswer } from "@/plugins/builtin/agent/public/hitl";
import {
  canSubmitQuestion,
  questionDraftAnswers,
  questionDraftComplete,
  questionSettled,
  type QuestionAnswers,
  type QuestionDraft,
} from "@/plugins/builtin/agent/public/messagePresentation";

export interface QuestionCardSettledView {
  settled: boolean;
  answers?: QuestionAnswers;
}

interface PendingQuestionRequest {
  status: "requires-action";
  runId?: string;
  itemId?: string;
  questions: QuestionItem[];
  answered?: boolean;
  answers?: string[][];
}

/** The single pending root question that temporarily owns the composer rung.
 * The transcript remains the durable source; this selector only chooses its
 * presentation location and never creates a second interrupt read model. */
export function pendingQuestionRequest(
  rows: readonly TranscriptRow[],
): PendingQuestionRequest | null {
  for (let rowIndex = rows.length - 1; rowIndex >= 0; rowIndex -= 1) {
    const blocks = rows[rowIndex]!.message.blocks;
    for (let blockIndex = blocks.length - 1; blockIndex >= 0; blockIndex -= 1) {
      const block = blocks[blockIndex]!;
      if (
        block.kind === "question" &&
        block.status === "requires-action" &&
        !block.answered &&
        block.questions.length > 0
      ) {
        return block as PendingQuestionRequest;
      }
    }
  }
  return null;
}

export function questionCardSettledView({
  status,
  answered,
  pending,
  answers,
}: {
  status: BlockStatus;
  answered?: boolean;
  pending: boolean;
  questions: readonly QuestionItem[];
  draft: QuestionDraft;
  answers?: QuestionAnswers;
}): QuestionCardSettledView {
  // Submission is only an in-flight intent. Keep the request surface disabled
  // until the Runtime stamps the authoritative answer into the transcript;
  // otherwise a skipped response briefly looks like a completed empty answer.
  if (pending) return { settled: false };
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
  pending,
}: {
  runId?: string;
  itemId?: string;
  status: BlockStatus;
  pending: boolean;
}): boolean {
  return !pending && canSubmitQuestion({ runId, itemId, status });
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

  const submitAnswer = useCallback(
    (submittedDraft: QuestionDraft = draft) => {
      submit(questionDraftAnswers(questions, submittedDraft));
    },
    [draft, questions, submit],
  );

  return {
    pending,
    complete,
    disabled: !canSubmitQuestionCard({ runId, itemId, status, pending }),
    submit: submitAnswer,
  };
}
