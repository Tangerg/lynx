import type { BlockStatus, QuestionItem } from "@/plugins/builtin/agent/public/viewState";
import { useState } from "react";
import { Button, Icon, Pressable, Surface, TextField } from "@/ui";
import { HitlCardShell, HitlSettledRow } from "./HitlCard";
import { useT } from "@/lib/i18n";
import {
  createQuestionDraft,
  questionAnswerText,
  setQuestionText,
  toggleQuestionOption,
  type QuestionDraft,
} from "@/plugins/builtin/agent/public/messagePresentation";
import {
  questionCardSettledView,
  useQuestionCardActions,
} from "../../application/questionCardModel";
import { cn } from "@/lib/classNames";

interface Props {
  /** Block lifecycle. `"requires-action"` shows the interactive card;
   *  `"complete"` (or `answered`) collapses to a settled row. */
  status: BlockStatus;
  /** The Run to resume + the question Item — the HITL resume target
   *  (API.md §6). Absent ⇒ decorative preview with no submit button. */
  runId?: string;
  itemId?: string;
  questions: QuestionItem[];
  /** Set once the answer is submitted (optimistic) / the run resolves. */
  answered?: boolean;
  /** Submitted values in Question.fields order, echoed on the settled card.
   *  Absent on history replay → the card falls back to a bare row. */
  answers?: string[][];
}

// Clarifying-question card — presentation shell. Submission coordination lives
// in useQuestionCardActions; this component owns the local selection draft.
//
// HITL flow (R-model, API.md §6; parallels ApprovalCard):
//   1. Run ends with a question Interrupt → reducer materialises a question
//      block (status="requires-action") bound to { runId, itemId }
//   2. User selects / types → useQuestionAnswer resumes the run (new segment)
//      via runs.resume + optimistically settles the card (resolveInterrupt)
export function QuestionCard({ status, runId, itemId, questions, answered, answers }: Props) {
  const t = useT();
  const [draft, setDraft] = useState<QuestionDraft>(() => createQuestionDraft(questions));
  const actions = useQuestionCardActions({ runId, itemId, status, questions, draft });

  const settled = questionCardSettledView({
    status,
    answered,
    pending: actions.pending,
    questions,
    draft,
    answers,
  });

  if (settled.settled) {
    const shown = settled.answers;
    if (!shown) return <HitlSettledRow label={t("question.settled.answered")} />;
    return (
      <Surface className="my-2 flex flex-col gap-2">
        <div className="flex items-center gap-1.5 font-mono text-ui-xs font-medium text-fg-faint">
          <Icon name="check" size="xs" />
          <span>{t("question.settled.answered")}</span>
        </div>
        {questions.map((q, index) => (
          <div key={index} className="flex flex-col gap-0.5">
            <div className="text-ui-md leading-snug text-fg-muted">{q.prompt}</div>
            <div className="text-ui-lg font-medium text-fg">
              {questionAnswerText(shown, index) || "—"}
            </div>
          </div>
        ))}
      </Surface>
    );
  }

  return (
    <HitlCardShell
      variant="neutral"
      icon="question"
      iconClassName="text-accent"
      label={t("question.required")}
    >
      <div className="flex flex-col gap-3">
        {questions.map((q, index) => {
          const cur = draft[index] ?? { selected: [], text: "" };
          return (
            <div key={index} className="flex flex-col gap-1.5">
              {(q.header || (q.type === "choice" && q.multiple)) && (
                <div className="flex items-center gap-2">
                  {q.header && (
                    <span className="rounded-sm bg-surface-2 px-1.5 py-px font-mono text-ui-xs font-semibold text-fg-muted">
                      {q.header}
                    </span>
                  )}
                  {q.type === "choice" && q.multiple && (
                    <span className="font-mono text-ui-xs text-fg-faint">
                      {t("question.multiSelect")}
                    </span>
                  )}
                </div>
              )}
              <div className="text-ui-lg font-semibold leading-body text-fg">{q.prompt}</div>

              {q.type === "choice" && (
                <div className="grid grid-cols-[minmax(0,1fr)] gap-1.5">
                  {q.options.map((opt) => {
                    const active = cur.selected.includes(opt.label);
                    return (
                      <Pressable
                        key={opt.label}
                        type="button"
                        aria-pressed={active}
                        onClick={() =>
                          setDraft((prev) => toggleQuestionOption(prev, index, q, opt.label))
                        }
                        className={cn(
                          "flex flex-col gap-0.5 rounded-md border-[0.5px] border-transparent px-2.5 py-1.5 text-left transition-colors duration-[var(--dur-fast)]",
                          active ? "border-accent/60 bg-accent-wash" : "bg-sunken hover:bg-hover",
                        )}
                      >
                        <span className="text-ui-md font-medium text-fg">{opt.label}</span>
                        {opt.description && (
                          <span className="text-ui-sm leading-body text-fg-muted">
                            {opt.description}
                          </span>
                        )}
                        {opt.preview && (
                          <code className="mt-1 block whitespace-pre-wrap break-all rounded-sm bg-surface-3 px-2 py-1 font-mono text-ui-sm text-fg-muted">
                            {opt.preview}
                          </code>
                        )}
                      </Pressable>
                    );
                  })}
                </div>
              )}

              {(q.type === "text" || q.allowCustom) && (
                <TextField
                  variant="bare"
                  font="sans"
                  value={cur.text}
                  aria-label={q.prompt}
                  placeholder={t("question.freetext.placeholder")}
                  onChange={(e) => {
                    setDraft((prev) => setQuestionText(prev, index, q, e.target.value));
                  }}
                  className="border-b-[0.5px] border-field py-1 text-display-sm focus:border-fg"
                />
              )}
            </div>
          );
        })}
      </div>

      <div className="mt-2.5 flex items-center gap-2">
        <Button variant="primary" size="sm" disabled={actions.disabled} onClick={actions.submit}>
          {t("question.action.submit")}
        </Button>
      </div>
    </HitlCardShell>
  );
}
