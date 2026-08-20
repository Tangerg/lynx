import type {
  BlockStatus,
  ChoiceQuestionItem,
  QuestionItem,
} from "@/plugins/sdk/types/contentBlock";

export type QuestionAnswers = string[][];

export interface QuestionDraftEntry {
  selected: string[];
  text: string;
}

export type QuestionDraft = QuestionDraftEntry[];

const EMPTY_ENTRY: QuestionDraftEntry = { selected: [], text: "" };

export function createQuestionDraft(questions: readonly QuestionItem[]): QuestionDraft {
  return questions.map((question) => ({
    selected:
      question.type === "choice" && !question.multiple && question.options[0]
        ? [question.options[0].label]
        : [],
    text: "",
  }));
}

export function questionDraftComplete(
  questions: readonly QuestionItem[],
  draft: QuestionDraft,
): boolean {
  return (
    questions.length > 0 &&
    questions.every((question, index) => isAnswered(question, draft[index] ?? EMPTY_ENTRY))
  );
}

export function questionAnswerText(answers: QuestionAnswers, index: number): string {
  return (answers[index] ?? []).filter(Boolean).join(", ");
}

export function questionSettled(status: BlockStatus, answered: boolean | undefined): boolean {
  return status === "complete" || Boolean(answered);
}

export function canSubmitQuestion({
  runId,
  itemId,
  status,
}: {
  runId?: string;
  itemId?: string;
  status: BlockStatus;
}): boolean {
  return Boolean(runId && itemId && status === "requires-action");
}

export function questionDraftAnswers(
  questions: readonly QuestionItem[],
  draft: QuestionDraft,
): QuestionAnswers {
  return questions.map((question, index) => {
    const { selected, text } = draft[index] ?? EMPTY_ENTRY;
    const trimmed = text.trim();
    if (question.type === "text") return trimmed ? [trimmed] : [];
    if (question.multiple) {
      const values = [...selected];
      if (question.allowCustom && trimmed && !values.includes(trimmed)) values.push(trimmed);
      return values;
    }
    const answer = question.allowCustom && trimmed ? trimmed : selected[0];
    return answer ? [answer] : [];
  });
}

export function clearQuestionAnswer(draft: QuestionDraft, index: number): QuestionDraft {
  return replaceQuestionDraftEntry(draft, index, { selected: [], text: "" });
}

export function toggleQuestionOption(
  draft: QuestionDraft,
  index: number,
  question: ChoiceQuestionItem,
  label: string,
): QuestionDraft {
  const current = draft[index] ?? EMPTY_ENTRY;
  if (question.multiple) {
    const selected = current.selected.includes(label)
      ? current.selected.filter((item) => item !== label)
      : [...current.selected, label];
    return replaceQuestionDraftEntry(draft, index, { ...current, selected });
  }

  return replaceQuestionDraftEntry(draft, index, { selected: [label], text: "" });
}

export function setQuestionText(
  draft: QuestionDraft,
  index: number,
  question: QuestionItem,
  text: string,
): QuestionDraft {
  if (question.type === "choice" && !question.allowCustom) return draft;
  const current = draft[index] ?? EMPTY_ENTRY;
  return replaceQuestionDraftEntry(draft, index, {
    selected: question.type === "choice" && question.multiple ? current.selected : [],
    text,
  });
}

function replaceQuestionDraftEntry(
  draft: QuestionDraft,
  index: number,
  entry: QuestionDraftEntry,
): QuestionDraft {
  const next = [...draft];
  next[index] = entry;
  return next;
}

function isAnswered(question: QuestionItem, entry: QuestionDraftEntry): boolean {
  return (
    entry.selected.length > 0 ||
    ((question.type === "text" || question.allowCustom) && entry.text.trim().length > 0)
  );
}
