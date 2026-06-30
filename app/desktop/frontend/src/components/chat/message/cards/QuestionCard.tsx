import type { BlockStatus, QuestionItem } from "@/protocol/run/viewState";
import { useMemo, useState } from "react";
import { Icon } from "@/components/common";
import { HitlCardShell, HitlSettledRow } from "./HitlCard";
import { useT } from "@/lib/i18n";
import { useQuestionAnswer, type QuestionAnswers } from "@/lib/agent/useQuestionAnswer";
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

// Per-question working state: chosen option labels + an optional free-text
// value. For single-select the two are mutually exclusive (picking an
// option clears the text and vice versa) so the submitted answer is never
// ambiguous; multi-select unions them.
type Draft = Record<string, { selected: string[]; text: string }>;

function emptyDraft(questions: QuestionItem[]): Draft {
  const d: Draft = {};
  for (const q of questions) d[q.id] = { selected: [], text: "" };
  return d;
}

function isAnswered(entry: { selected: string[]; text: string }): boolean {
  return entry.selected.length > 0 || entry.text.trim().length > 0;
}

// Flatten one question's answer (string | string[], from either the block or a
// freshly-projected draft) into a single display line.
function answerText(answers: QuestionAnswers, id: string): string {
  const v = answers[id];
  if (v == null) return "";
  return (Array.isArray(v) ? v : [v]).filter(Boolean).join(", ");
}

// Project the working draft into the wire shape: question.id → label(s).
// Free text (when present) is the answer itself, not an "Other" marker
// (API.md §4.4); multi-select appends it to the chosen labels.
function toAnswers(questions: QuestionItem[], draft: Draft): QuestionAnswers {
  const answers: QuestionAnswers = {};
  for (const q of questions) {
    const { selected, text } = draft[q.id] ?? { selected: [], text: "" };
    const trimmed = text.trim();
    if (q.multiSelect) {
      answers[q.id] = trimmed ? [...selected, trimmed] : selected;
    } else {
      answers[q.id] = trimmed || (selected[0] ?? "");
    }
  }
  return answers;
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
  const [draft, setDraft] = useState<Draft>(() => emptyDraft(questions));

  const settled = status === "complete" || answered;
  const allAnswered = useMemo(
    () => questions.every((q) => isAnswered(draft[q.id] ?? { selected: [], text: "" })),
    [questions, draft],
  );

  if (settled || pending) {
    // Echo what was answered: the stamped block answers (survive remount) or,
    // in the brief pre-settle window, the local draft. Replayed-from-history
    // questions carry neither → a bare "answered" row.
    const shown: QuestionAnswers | undefined =
      answers ?? (allAnswered ? toAnswers(questions, draft) : undefined);
    if (!shown) return <HitlSettledRow label={t("question.settled.answered")} />;
    return (
      <div className="my-3 flex flex-col gap-2 rounded-md border border-line bg-surface px-4 py-3">
        <div className="flex items-center gap-1.5 font-mono text-[10px] font-medium text-fg-faint">
          <Icon name="check" size={11} strokeWidth={3} />
          <span>{t("question.settled.answered")}</span>
        </div>
        {questions.map((q) => (
          <div key={q.id} className="flex flex-col gap-0.5">
            <div className="text-[12.5px] leading-snug text-fg-muted">{q.question}</div>
            <div className="text-[13px] font-medium text-fg">{answerText(shown, q.id) || "—"}</div>
          </div>
        ))}
      </div>
    );
  }

  const toggleOption = (q: QuestionItem, label: string) => {
    setDraft((prev) => {
      const cur = prev[q.id] ?? { selected: [], text: "" };
      if (q.multiSelect) {
        const selected = cur.selected.includes(label)
          ? cur.selected.filter((l) => l !== label)
          : [...cur.selected, label];
        return { ...prev, [q.id]: { ...cur, selected } };
      }
      // Single-select: replace selection, clear any free text.
      return { ...prev, [q.id]: { selected: [label], text: "" } };
    });
  };

  const setText = (q: QuestionItem, text: string) => {
    setDraft((prev) => {
      const cur = prev[q.id] ?? { selected: [], text: "" };
      // Typing free text in a single-select clears the option choice.
      return { ...prev, [q.id]: { selected: q.multiSelect ? cur.selected : [], text } };
    });
  };

  // Also disable once the interrupt is no longer open: settleOpenInterrupts
  // downgrades an unacted question to `incomplete` on run-end so its Submit
  // can't resume a dead run.
  const disabled = !parentRunId || !itemId || !allAnswered || status !== "requires-action";

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
                      onClick={() => toggleOption(q, opt.label)}
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
                  onChange={(e) => setText(q, e.target.value)}
                  className="w-full bg-transparent border-b border-line py-1 text-[16px] text-fg placeholder:text-fg-faint outline-none focus:border-fg"
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
          onClick={() => submit(toAnswers(questions, draft))}
          className="inline-flex cursor-pointer items-center rounded-md bg-fg px-3 py-1.5 text-[13px] font-medium text-on-fg transition-opacity duration-150 ease-out hover:opacity-90 disabled:cursor-not-allowed disabled:opacity-50"
        >
          {t("question.action.submit")}
        </button>
      </div>
    </HitlCardShell>
  );
}
