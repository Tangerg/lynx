import type { BlockStatus, QuestionItem } from "@/plugins/builtin/agent/public/viewState";
import { useId, useState, type KeyboardEvent } from "react";
import { Button, Icon, Pressable, Surface, TextArea, TextField } from "@/ui";
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
import { useRuntimeCommandsAvailable } from "@/plugins/builtin/runtime/public/serviceStatus";

interface Props {
  /** Block lifecycle. `"requires-action"` shows the interactive card;
   *  `"complete"` (or `answered`) collapses to a settled row. */
  status: BlockStatus;
  /** The Run to resume + the question Item — the HITL resume target
   *  (API.md §6). Absent ⇒ decorative preview with no submit button. */
  runId?: string;
  itemId?: string;
  questions: QuestionItem[];
  /** Runtime-projected accepted-answer state. */
  answered?: boolean;
  /** Runtime-projected accepted values in Question.fields order. */
  answers?: string[][];
}

// Clarifying-question card — presentation shell. Submission coordination lives
// in useQuestionCardActions; this component owns the local selection draft.
//
// HITL flow (R-model, API.md §6; parallels ApprovalCard):
//   1. Run ends with a question Interrupt → reducer materialises a question
//      block (status="requires-action") bound to { runId, itemId }
//   2. User selects / types → useQuestionAnswer resumes the run (new segment)
//   3. The accepted response returns on the authoritative Question transcript
//      projection; an unaccepted/canceled question has no answers.
export function QuestionCard({ status, runId, itemId, questions, answered, answers }: Props) {
  const t = useT();
  const questionCardId = useId();
  const runtimeAvailable = useRuntimeCommandsAvailable();
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
    if (!shown) return <HitlSettledRow label={t("question.settled.dismissed")} />;
    return (
      <Surface className="flex flex-col gap-2">
        <div className="flex items-center gap-1.5 font-mono text-ui-xs font-medium text-fg-faint">
          <Icon name="check" size="xs" />
          <span>{t("question.settled.answered")}</span>
        </div>
        {questions.map((q, index) => (
          <div key={index} className="flex flex-col gap-0.5">
            <div className="text-ui-md leading-snug text-fg-muted">{q.prompt}</div>
            <div className="text-ui-md font-medium text-fg">
              {questionAnswerText(shown, index) || "—"}
            </div>
          </div>
        ))}
      </Surface>
    );
  }

  const handleSingleChoiceKeyDown = (
    event: KeyboardEvent<HTMLButtonElement>,
    questionIndex: number,
    question: QuestionItem,
    optionIndex: number,
  ) => {
    if (question.type !== "choice" || question.multiple) return;

    let nextIndex: number | undefined;
    if (event.key === "ArrowDown" || event.key === "ArrowRight") {
      nextIndex = (optionIndex + 1) % question.options.length;
    } else if (event.key === "ArrowUp" || event.key === "ArrowLeft") {
      nextIndex = (optionIndex - 1 + question.options.length) % question.options.length;
    } else if (event.key === "Home") {
      nextIndex = 0;
    } else if (event.key === "End") {
      nextIndex = question.options.length - 1;
    } else if (/^[1-9]$/.test(event.key)) {
      const numberedIndex = Number(event.key) - 1;
      if (numberedIndex < question.options.length) nextIndex = numberedIndex;
    }

    if (nextIndex === undefined) return;
    event.preventDefault();
    const next = question.options[nextIndex];
    if (!next) return;
    setDraft((previous) => toggleQuestionOption(previous, questionIndex, question, next.label));
    const options =
      event.currentTarget.parentElement?.querySelectorAll<HTMLElement>('[role="radio"]');
    options?.[nextIndex]?.focus();
  };

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
          const promptId = `${questionCardId}-prompt-${index}`;
          const selectedPreview =
            q.type === "choice" && !q.multiple
              ? q.options.find((option) =>
                  cur.selected.includes(option.label) && option.preview ? true : false,
                )
              : undefined;
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
              <div id={promptId} className="text-ui-md font-semibold leading-body text-fg">
                {q.prompt}
              </div>

              {q.type === "choice" && (
                <div
                  className={cn(
                    "grid grid-cols-[minmax(0,1fr)] gap-2",
                    selectedPreview?.preview &&
                      "@min-[640px]:grid-cols-[minmax(0,1fr)_minmax(0,1fr)]",
                  )}
                >
                  <div
                    role={q.multiple ? "group" : "radiogroup"}
                    aria-labelledby={promptId}
                    className="grid grid-cols-[minmax(0,1fr)] gap-1"
                  >
                    {q.options.map((opt, optionIndex) => {
                      const active = cur.selected.includes(opt.label);
                      return (
                        <Pressable
                          key={opt.label}
                          type="button"
                          role={q.multiple ? "checkbox" : "radio"}
                          aria-checked={active}
                          tabIndex={
                            q.multiple || active || (!cur.selected.length && optionIndex === 0)
                              ? 0
                              : -1
                          }
                          onClick={() =>
                            setDraft((prev) => toggleQuestionOption(prev, index, q, opt.label))
                          }
                          onKeyDown={(event) =>
                            handleSingleChoiceKeyDown(event, index, q, optionIndex)
                          }
                          className={cn(
                            "group/choice flex min-h-8 items-start gap-2 rounded-md px-2 py-1.5 text-left transition-colors duration-[var(--dur-fast)]",
                            active ? "bg-hover" : "hover:bg-hover",
                          )}
                        >
                          <span
                            aria-hidden
                            className={cn(
                              "mt-0.5 grid size-4 shrink-0 place-items-center border text-ui-xs leading-none font-medium",
                              q.multiple ? "rounded-2xs" : "rounded-full",
                              active
                                ? "border-accent bg-accent text-on-accent"
                                : "border-field text-fg-muted",
                            )}
                          >
                            {active && q.multiple ? (
                              <Icon name="check" size="xs" />
                            ) : active ? (
                              <span className="size-1.5 rounded-full bg-current" />
                            ) : q.multiple ? null : (
                              optionIndex + 1
                            )}
                          </span>
                          <span className="flex min-w-0 flex-1 items-baseline gap-2">
                            <span className="min-w-0 max-w-1/2 shrink-0 truncate text-ui-md font-medium text-fg">
                              {opt.label}
                            </span>
                            {opt.description && (
                              <span
                                title={opt.description}
                                className="min-w-0 flex-1 truncate text-ui-sm leading-body text-fg-muted"
                              >
                                {opt.description}
                              </span>
                            )}
                          </span>
                        </Pressable>
                      );
                    })}
                  </div>
                  {selectedPreview?.preview && (
                    <div
                      role="region"
                      aria-label={selectedPreview.label}
                      className="min-w-0 rounded-md bg-sunken p-2.5"
                    >
                      <code className="block whitespace-pre-wrap break-all font-mono text-ui-sm leading-body text-fg-muted">
                        {selectedPreview.preview}
                      </code>
                    </div>
                  )}
                </div>
              )}

              {q.type === "text" && (
                <TextArea
                  font="sans"
                  size="sm"
                  rows={4}
                  value={cur.text}
                  aria-label={q.prompt}
                  placeholder={t("question.freetext.placeholder")}
                  onChange={(event) => {
                    setDraft((previous) => setQuestionText(previous, index, q, event.target.value));
                  }}
                  className="max-h-40"
                />
              )}

              {q.type === "choice" && q.allowCustom && (
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
        <Button
          variant="primary"
          size="sm"
          disabled={actions.disabled || !runtimeAvailable}
          onClick={actions.submit}
        >
          {t("question.action.submit")}
        </Button>
      </div>
    </HitlCardShell>
  );
}
