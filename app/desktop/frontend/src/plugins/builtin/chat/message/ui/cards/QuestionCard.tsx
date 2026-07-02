import type { BlockStatus, QuestionItem } from "@/plugins/builtin/agent/public/viewState";
import { useMemo, useState } from "react";
import { Icon } from "@/components/common";
import { HitlCardShell, HitlSettledRow } from "./HitlCard";
import { useT } from "@/lib/i18n";
import { useQuestionAnswer } from "@/plugins/builtin/agent/public/hitl";
import {
  createQuestionDraft,
  questionAnswerText,
  questionDraftAnswers,
  questionDraftComplete,
  questionSettled,
  questionSettledAnswers,
  setQuestionText,
  toggleQuestionOption,
  canSubmitQuestion,
  type QuestionDraft,
  type QuestionAnswers,
} from "@/plugins/builtin/agent/public/messagePresentation";
import { cn } from "@/lib/utils";

interface Props {
  /** Block lifecycle. `"requires-action"` shows the interactive card;
   *  `"complete"` (or `answered`) collapses to a settled row. */
  status: BlockStatus;
  /** The interrupted Run + the question Item — the HITL resume target
   *  (API.md §6). Absent ⇒ decorative preview with no submit button. */
  parentRunId?: string;
  itemId?: string;
  questions: QuestionItem[];
  /** Set once the answer is submitted (optimistic) / the run resolves. */
  answered?: boolean;
  /** The submitted answer (QuestionItem.id → labels), echoed on the settled
   *  card. Absent on history replay → the card falls back to a bare row. */
  answers?: Record<string, string[]>;
}

// Clarifying-question card — pure presentation. Submitting state lives in
// useQuestionAnswer; this component owns the local selection draft.
//
// HITL flow (R-model, API.md §6; parallels ApprovalCard):
//   1. Run ends with a question Interrupt → reducer materialises a question
//      block (status="requires-action") bound to { parentRunId, itemId }
//   2. User selects / types → useQuestionAnswer starts a continuation Run
//      via runs.resume + optimistically settles the card (resolveInterrupt)
export function QuestionCard({ status, parentRunId, itemId, questions, answered, answers }: Props) {
  const t = useT();
  const { submit, pending } = useQuestionAnswer(parentRunId, itemId);
  const [draft, setDraft] = useState<QuestionDraft>(() => createQuestionDraft(questions));

  const settled = questionSettled(status, answered);
  const allAnswered = useMemo(() => questionDraftComplete(questions, draft), [questions, draft]);

  if (settled || pending) {
    // Echo what was answered: the stamped block answers (survive remount) or,
    // in the brief pre-settle window, the local draft. Replayed-from-history
    // questions carry neither → a bare "answered" row.
    const shown: QuestionAnswers | undefined = questionSettledAnswers(questions, draft, answers);
    if (!shown) return <HitlSettledRow label={t("question.settled.answered")} />;
    return (
      <div className="my-3 flex flex-col gap-2 rounded-md bg-surface px-4 py-3 shadow-[var(--shadow-surface)]">
        <div className="flex items-center gap-1.5 font-mono text-[10px] font-medium text-fg-faint">
          <Icon name="check" size={11} strokeWidth={3} />
          <span>{t("question.settled.answered")}</span>
        </div>
        {questions.map((q) => (
          <div key={q.id} className="flex flex-col gap-0.5">
            <div className="text-[12.5px] leading-snug text-fg-muted">{q.question}</div>
            <div className="text-[13px] font-medium text-fg">
              {questionAnswerText(shown, q.id) || "—"}
            </div>
          </div>
        ))}
      </div>
    );
  }

  // Also disable once the interrupt is no longer open: settleOpenInterrupts
  // downgrades an unacted question to `incomplete` on run-end so its Submit
  // can't resume a dead run.
  const disabled = !canSubmitQuestion({
    parentRunId,
    itemId,
    complete: allAnswered,
    status,
  });

  return (
    <HitlCardShell
      data-slot="question-card"
      variant="neutral"
      icon="question"
      iconClassName="text-accent"
      label={t("question.required")}
    >
      <div className="flex flex-col gap-4">
        {questions.map((q) => {
          const cur = draft[q.id] ?? { selected: [], text: "" };
          return (
            <div key={q.id} className="flex flex-col gap-2">
              <div className="flex items-center gap-2">
                <span className="rounded-sm border border-line bg-surface-2 px-1.5 py-px font-mono text-[10px] font-semibold text-fg-muted">
                  {q.header}
                </span>
                {q.multiSelect && (
                  <span className="font-mono text-[10px] text-fg-faint">
                    {t("question.multiSelect")}
                  </span>
                )}
              </div>
              <div className="text-[16px] font-semibold leading-[1.4] text-fg">{q.question}</div>

              <div className="grid grid-cols-[minmax(0,1fr)] gap-1.5">
                {q.options.map((opt) => {
                  const active = cur.selected.includes(opt.label);
                  return (
                    <button
                      key={opt.label}
                      type="button"
                      aria-pressed={active}
                      onClick={() => setDraft((prev) => toggleQuestionOption(prev, q, opt.label))}
                      className={cn(
                        "flex flex-col gap-0.5 rounded-md border px-3 py-2 text-left transition-colors duration-150",
                        active
                          ? "border-accent/60 bg-accent/10"
                          : "border-line bg-surface-2 hover:border-line-soft hover:bg-surface-3",
                      )}
                    >
                      <span className="text-[13px] font-medium text-fg">{opt.label}</span>
                      {opt.description && (
                        <span className="text-[12px] leading-[1.45] text-fg-muted">
                          {opt.description}
                        </span>
                      )}
                      {opt.preview && (
                        <code className="mt-1 block whitespace-pre-wrap break-all rounded-sm bg-surface-3 px-2 py-1 font-mono text-[11px] text-fg-muted">
                          {opt.preview}
                        </code>
                      )}
                    </button>
                  );
                })}
              </div>

              {q.allowFreeText && (
                <input
                  type="text"
                  data-slot="question-input"
                  value={cur.text}
                  aria-label={q.question}
                  placeholder={t("question.freetext.placeholder")}
                  onChange={(e) => {
                    setDraft((prev) => setQuestionText(prev, q, e.target.value));
                  }}
                  className="w-full bg-transparent border-b border-field py-1 text-[16px] text-fg placeholder:text-fg-faint outline-none focus:border-fg"
                />
              )}
            </div>
          );
        })}
      </div>

      <div className="mt-3.5 flex items-center gap-2">
        <button
          type="button"
          data-slot="question-submit"
          disabled={disabled}
          onClick={() => submit(questionDraftAnswers(questions, draft))}
          className="inline-flex items-center rounded-md bg-fg px-3 py-1.5 text-[13px] font-medium text-on-fg transition-opacity duration-150 ease-out hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {t("question.action.submit")}
        </button>
      </div>
    </HitlCardShell>
  );
}
