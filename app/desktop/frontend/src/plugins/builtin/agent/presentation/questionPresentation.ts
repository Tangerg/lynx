import type { BlockStatus, QuestionItem } from "@/plugins/sdk/types/contentBlock";

export type QuestionAnswers = Record<string, string | string[]>;

export interface QuestionDraftEntry {
  selected: string[];
  text: string;
}

export type QuestionDraft = Record<string, QuestionDraftEntry>;

const EMPTY_ENTRY: QuestionDraftEntry = { selected: [], text: "" };

export function createQuestionDraft(questions: readonly QuestionItem[]): QuestionDraft {
  const draft: QuestionDraft = {};
  for (const question of questions) draft[question.id] = { selected: [], text: "" };
  return draft;
}

export function questionDraftComplete(
  questions: readonly QuestionItem[],
  draft: QuestionDraft,
): boolean {
  return questions.every((question) => isAnswered(draft[question.id] ?? EMPTY_ENTRY));
}

export function questionAnswerText(answers: QuestionAnswers, id: string): string {
  const value = answers[id];
  if (value == null) return "";
  return (Array.isArray(value) ? value : [value]).filter(Boolean).join(", ");
}

export function questionSettled(status: BlockStatus, answered: boolean | undefined): boolean {
  return status === "complete" || Boolean(answered);
}

export function questionSettledAnswers(
  questions: readonly QuestionItem[],
  draft: QuestionDraft,
  answers: QuestionAnswers | undefined,
): QuestionAnswers | undefined {
  return (
    answers ??
    (questionDraftComplete(questions, draft) ? questionDraftAnswers(questions, draft) : undefined)
  );
}

export function canSubmitQuestion({
  runId,
  itemId,
  complete,
  status,
}: {
  runId?: string;
  itemId?: string;
  complete: boolean;
  status: BlockStatus;
}): boolean {
  return Boolean(runId && itemId && complete && status === "requires-action");
}

export function questionDraftAnswers(
  questions: readonly QuestionItem[],
  draft: QuestionDraft,
): QuestionAnswers {
  const answers: QuestionAnswers = {};
  for (const question of questions) {
    const { selected, text } = draft[question.id] ?? EMPTY_ENTRY;
    const trimmed = text.trim();
    if (question.multiSelect) {
      answers[question.id] = trimmed ? [...selected, trimmed] : selected;
    } else {
      answers[question.id] = trimmed || (selected[0] ?? "");
    }
  }
  return answers;
}

export function toggleQuestionOption(
  draft: QuestionDraft,
  question: QuestionItem,
  label: string,
): QuestionDraft {
  const current = draft[question.id] ?? EMPTY_ENTRY;
  if (question.multiSelect) {
    const selected = current.selected.includes(label)
      ? current.selected.filter((item) => item !== label)
      : [...current.selected, label];
    return { ...draft, [question.id]: { ...current, selected } };
  }

  return { ...draft, [question.id]: { selected: [label], text: "" } };
}

export function setQuestionText(
  draft: QuestionDraft,
  question: QuestionItem,
  text: string,
): QuestionDraft {
  const current = draft[question.id] ?? EMPTY_ENTRY;
  return {
    ...draft,
    [question.id]: { selected: question.multiSelect ? current.selected : [], text },
  };
}

function isAnswered(entry: QuestionDraftEntry): boolean {
  return entry.selected.length > 0 || entry.text.trim().length > 0;
}
