import type { BlockStatus, QuestionItem } from "@/plugins/builtin/agent/public/viewState";
import {
  useId,
  useLayoutEffect,
  useRef,
  useState,
  type ChangeEvent,
  type CompositionEvent,
  type KeyboardEvent,
} from "react";
import { Button, Icon, IconButton, Pressable, Surface, TextArea, TextField } from "@/ui";
import { AgentActivityDisclosure } from "@/ui/agent";
import { HitlSettledRow } from "./HitlCard";
import { useT } from "@/lib/i18n";
import {
  clearQuestionAnswer,
  createQuestionDraft,
  questionAnswerText,
  questionDraftComplete,
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
import { composerCompositionKeyIntent } from "@/plugins/builtin/chat/composer/public/composition";

interface Props {
  status: BlockStatus;
  runId?: string;
  itemId?: string;
  questions: QuestionItem[];
  answered?: boolean;
  answers?: string[][];
}

const RECOMMENDED_SUFFIX = " (Recommended)";

/** Codex-style native question request. The durable Question block remains the
 * only fact owner; while pending, ChatStream places this surface on the composer
 * rung and suppresses the block's duplicate transcript presentation. */
export function QuestionCard({ status, runId, itemId, questions, answered, answers }: Props) {
  const t = useT();
  const questionCardId = useId();
  const runtimeAvailable = useRuntimeCommandsAvailable();
  const [draft, setDraft] = useState<QuestionDraft>(() => createQuestionDraft(questions));
  const [questionIndex, setQuestionIndex] = useState(0);
  const [settledOpen, setSettledOpen] = useState(false);
  const requestRef = useRef<HTMLDivElement>(null);
  const focusQuestionOnChange = useRef(false);
  const activeQuestionRef = useRef<HTMLDivElement>(null);
  const composingRef = useRef(false);
  const compositionCommitPendingRef = useRef(false);
  const actions = useQuestionCardActions({ runId, itemId, status, questions, draft });
  const activeIndex = Math.min(questionIndex, Math.max(questions.length - 1, 0));
  const activeQuestion = questions[activeIndex];
  const activeDraft = draft[activeIndex] ?? { selected: [], text: "" };
  const activeQuestionComplete = activeQuestion
    ? questionDraftComplete([activeQuestion], [activeDraft])
    : false;
  const isLastQuestion = activeIndex >= questions.length - 1;

  useLayoutEffect(() => {
    if (!focusQuestionOnChange.current) {
      requestRef.current?.focus();
      return;
    }
    focusQuestionOnChange.current = false;
    activeQuestionRef.current
      ?.querySelector<HTMLElement>('[role="radio"], [role="checkbox"], textarea, input')
      ?.focus();
  }, [activeIndex]);

  const navigateQuestion = (nextIndex: number) => {
    const bounded = Math.min(Math.max(nextIndex, 0), Math.max(questions.length - 1, 0));
    if (bounded === activeIndex) return;
    composingRef.current = false;
    compositionCommitPendingRef.current = false;
    focusQuestionOnChange.current = true;
    setQuestionIndex(bounded);
  };

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
    const count = questions.length;
    const countLabel = t(
      count === 1 ? "question.settled.question.one" : "question.settled.question.other",
      { count },
    );
    return (
      <AgentActivityDisclosure
        icon="question"
        shell="line"
        open={settledOpen}
        onToggle={() => setSettledOpen((open) => !open)}
        label={
          <span className="flex min-w-0 items-center gap-1 truncate">
            <span className="text-fg-muted">{t("question.settled.asked")}</span>
            <span className="text-fg-faint">{countLabel}</span>
          </span>
        }
        contentClassName="pt-1 pb-0.5"
      >
        <div className="flex flex-col gap-3">
          {questions.map((question, index) => (
            <div key={index} className="flex flex-col gap-1">
              <div className="whitespace-pre-wrap text-ui-sm leading-4 text-fg-muted">
                {question.prompt}
              </div>
              <div className="whitespace-pre-wrap break-words text-ui-sm leading-4 text-fg-faint">
                {questionAnswerText(shown, index) || t("question.settled.noAnswer")}
              </div>
            </div>
          ))}
        </div>
      </AgentActivityDisclosure>
    );
  }

  const submitOrAdvance = (nextDraft: QuestionDraft) => {
    setDraft(nextDraft);
    if (isLastQuestion) {
      if (runtimeAvailable && !actions.disabled) actions.submit(nextDraft);
      return;
    }
    navigateQuestion(activeIndex + 1);
  };

  const chooseOption = (question: Extract<QuestionItem, { type: "choice" }>, label: string) => {
    if (!runtimeAvailable || actions.pending) return;
    const nextDraft = toggleQuestionOption(draft, activeIndex, question, label);
    if (question.multiple) {
      setDraft(nextDraft);
      return;
    }
    submitOrAdvance(nextDraft);
  };

  const skipCurrent = () => {
    if (!runtimeAvailable || actions.pending) return;
    submitOrAdvance(clearQuestionAnswer(draft, activeIndex));
  };

  const advanceCurrent = () => {
    if (!runtimeAvailable || actions.pending) return;
    submitOrAdvance(draft);
  };

  const handleSingleChoiceKeyDown = (
    event: KeyboardEvent<HTMLButtonElement>,
    question: Extract<QuestionItem, { type: "choice" }>,
    optionIndex: number,
  ) => {
    if (question.multiple) return;

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
      if (numberedIndex < question.options.length) {
        event.preventDefault();
        const numbered = question.options[numberedIndex];
        if (numbered) chooseOption(question, numbered.label);
      }
      return;
    }

    if (nextIndex === undefined) return;
    event.preventDefault();
    const next = question.options[nextIndex];
    if (!next) return;
    setDraft((previous) => toggleQuestionOption(previous, activeIndex, question, next.label));
    event.currentTarget.parentElement
      ?.querySelectorAll<HTMLElement>('[role="radio"]')
      ?.[nextIndex]?.focus();
  };

  const handleCompositionStart = () => {
    composingRef.current = true;
    compositionCommitPendingRef.current = false;
  };

  const handleAnswerChange = (event: ChangeEvent<HTMLTextAreaElement | HTMLInputElement>) => {
    if (!activeQuestion) return;
    const value = event.currentTarget.value;
    const nativeComposing = (event.nativeEvent as { isComposing?: boolean }).isComposing === true;
    if (composingRef.current && !nativeComposing) {
      composingRef.current = false;
      compositionCommitPendingRef.current = true;
    }
    setDraft((previous) => setQuestionText(previous, activeIndex, activeQuestion, value));
  };

  const handleCompositionEnd = (
    event: CompositionEvent<HTMLTextAreaElement | HTMLInputElement>,
  ) => {
    composingRef.current = false;
    compositionCommitPendingRef.current = true;
    if (!activeQuestion) return;
    const value = event.currentTarget.value;
    setDraft((previous) => setQuestionText(previous, activeIndex, activeQuestion, value));
  };

  const handleAnswerKeyDown = (event: KeyboardEvent<HTMLTextAreaElement | HTMLInputElement>) => {
    const compositionIntent = composerCompositionKeyIntent(
      event.nativeEvent,
      composingRef.current,
      compositionCommitPendingRef.current,
    );
    compositionCommitPendingRef.current = false;
    if (compositionIntent !== null) {
      if (compositionIntent === "committed-enter") event.preventDefault();
      return;
    }
    if (event.key !== "Enter" || event.shiftKey || event.altKey || event.ctrlKey || event.metaKey) {
      return;
    }
    event.preventDefault();
    if (activeQuestionComplete) advanceCurrent();
    else skipCurrent();
  };

  if (!activeQuestion) return null;

  const promptId = `${questionCardId}-prompt-${activeIndex}`;
  const isSingleChoice = activeQuestion.type === "choice" && !activeQuestion.multiple;
  const explicitFreeform = activeDraft.text.trim().length > 0;
  const explicitMultiChoice =
    activeQuestion.type === "choice" && activeQuestion.multiple && activeDraft.selected.length > 0;
  const actionSkips = isSingleChoice || (!explicitFreeform && !explicitMultiChoice);

  return (
    <Surface
      ref={requestRef}
      inset="none"
      tabIndex={0}
      data-slot="question-request-surface"
      data-chrome-focus
      className="overflow-hidden rounded-3xl shadow-[var(--shadow-popover)] outline-none"
    >
      <div className="flex items-start justify-between gap-3 px-4 pt-4 pb-2">
        <h3
          id={promptId}
          className="min-w-0 text-pretty text-ui-md font-medium leading-body text-fg"
        >
          {activeQuestion.prompt}
        </h3>
        {questions.length > 1 && (
          <div className="flex shrink-0 items-center gap-1 text-ui-xs text-fg-faint">
            <IconButton
              icon="chevron-left"
              size="xs"
              quiet
              disabled={activeIndex === 0}
              title={t("question.action.previous")}
              onClick={() => navigateQuestion(activeIndex - 1)}
            />
            <span className="min-w-10 text-center tabular-nums">
              {t("question.progress", { current: activeIndex + 1, total: questions.length })}
            </span>
            <IconButton
              icon="chevron-right"
              size="xs"
              quiet
              disabled={isLastQuestion}
              title={t("question.action.next")}
              onClick={() => navigateQuestion(activeIndex + 1)}
            />
          </div>
        )}
      </div>

      <div ref={activeQuestionRef} className="flex flex-col gap-1 px-2 pt-1 pb-2">
        {activeQuestion.type === "choice" && (
          <div
            role={activeQuestion.multiple ? "group" : "radiogroup"}
            aria-labelledby={promptId}
            className="flex flex-col gap-1"
          >
            {activeQuestion.options.map((option, optionIndex) => {
              const active = activeDraft.selected.includes(option.label);
              const recommended = option.label.endsWith(RECOMMENDED_SUFFIX);
              const label = recommended
                ? option.label.slice(0, -RECOMMENDED_SUFFIX.length)
                : option.label;
              return (
                <Pressable
                  key={option.label}
                  type="button"
                  role={activeQuestion.multiple ? "checkbox" : "radio"}
                  aria-label={option.label}
                  aria-description={option.description || undefined}
                  aria-checked={active}
                  disabled={!runtimeAvailable || actions.pending}
                  tabIndex={
                    activeQuestion.multiple ||
                    active ||
                    (!activeDraft.selected.length && optionIndex === 0)
                      ? 0
                      : -1
                  }
                  onClick={() => chooseOption(activeQuestion, option.label)}
                  onKeyDown={(event) =>
                    handleSingleChoiceKeyDown(event, activeQuestion, optionIndex)
                  }
                  className={cn(
                    "group/choice flex min-h-8 w-full items-center gap-2 rounded-full px-2 py-1.5 text-left outline-none transition-colors duration-[var(--dur-fast)] focus-visible:ring-2 focus-visible:ring-focus disabled:cursor-not-allowed disabled:opacity-64",
                    active ? "bg-hover" : "hover:bg-hover",
                  )}
                >
                  <span
                    aria-hidden
                    className={cn(
                      "grid size-5 shrink-0 place-items-center rounded-full border text-ui-xs leading-none font-medium",
                      active
                        ? "border-fg bg-fg text-canvas"
                        : "border-field bg-surface-2 text-fg-muted",
                    )}
                  >
                    {active && activeQuestion.multiple ? (
                      <Icon name="check" size="xs" />
                    ) : active ? (
                      <span className="size-1.5 rounded-full bg-current" />
                    ) : activeQuestion.multiple ? null : (
                      optionIndex + 1
                    )}
                  </span>
                  <span className="flex min-w-0 flex-1 items-baseline gap-2">
                    <span className="min-w-0 max-w-1/2 shrink-0 truncate text-ui-md font-medium text-fg">
                      {label}
                    </span>
                    {recommended && (
                      <span className="shrink-0 rounded-full bg-surface-2 px-1.5 py-0.5 text-ui-xs text-fg-muted">
                        {t("question.recommended")}
                      </span>
                    )}
                    {option.description && (
                      <span
                        title={option.description}
                        className="min-w-0 flex-1 truncate text-ui-sm leading-body text-fg-muted"
                      >
                        {option.description}
                      </span>
                    )}
                  </span>
                </Pressable>
              );
            })}
          </div>
        )}

        {activeQuestion.type === "text" && (
          <div className="px-2 py-1.5">
            <TextArea
              font="sans"
              size="sm"
              rows={4}
              value={activeDraft.text}
              aria-label={activeQuestion.prompt}
              placeholder={t("question.freetext.placeholder")}
              disabled={!runtimeAvailable || actions.pending}
              onChange={handleAnswerChange}
              onCompositionStart={handleCompositionStart}
              onCompositionEnd={handleCompositionEnd}
              onKeyDown={handleAnswerKeyDown}
              onBlur={() => {
                compositionCommitPendingRef.current = false;
              }}
              className="max-h-40"
            />
          </div>
        )}

        {activeQuestion.type === "choice" && activeQuestion.allowCustom && (
          <div className="flex min-h-8 items-center gap-2 rounded-full px-2 py-1.5 focus-within:ring-1 focus-within:ring-focus">
            <span
              aria-hidden
              className={cn(
                "grid size-5 shrink-0 place-items-center rounded-full border",
                explicitFreeform
                  ? "border-fg bg-fg text-canvas"
                  : "border-field bg-surface-2 text-fg-muted",
              )}
            >
              <Icon name="edit" size="xs" />
            </span>
            <TextField
              variant="bare"
              font="sans"
              value={activeDraft.text}
              aria-label={activeQuestion.prompt}
              placeholder={t("question.freetext.placeholder")}
              disabled={!runtimeAvailable || actions.pending}
              onChange={handleAnswerChange}
              onCompositionStart={handleCompositionStart}
              onCompositionEnd={handleCompositionEnd}
              onKeyDown={handleAnswerKeyDown}
              onBlur={() => {
                compositionCommitPendingRef.current = false;
              }}
              className="h-5 p-0 text-ui-md leading-body"
            />
          </div>
        )}

        <div className="flex items-center justify-end gap-2 px-2 py-1">
          <Button
            variant={actionSkips ? "outline" : "primary"}
            size="sm"
            disabled={actions.disabled || !runtimeAvailable}
            onClick={actionSkips ? skipCurrent : advanceCurrent}
          >
            {t(actionSkips ? "question.action.skip" : "question.action.advance")}
          </Button>
        </div>
      </div>
    </Surface>
  );
}
