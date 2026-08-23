import { useRef, useState, type FormEvent } from "react";

import type {
  Interrupt,
  InterruptResponse,
  PendingInterruptSet,
} from "@lyra/runtime-contract";

import { useLocalization, type Translate } from "../localization/Localization";
import { ApprovalInterrupt } from "./ApprovalInterrupt";
import { QuestionInterrupt } from "./QuestionInterrupt";
import {
  buildInterruptResponses,
  createInterruptDrafts,
  InterruptResponseValidationError,
  type InterruptDraft,
} from "./interruptResponse";

interface InterruptSetCardProps {
  interruptSet: PendingInterruptSet;
  pending: boolean;
  error?: string;
  onResume(
    interruptSet: PendingInterruptSet,
    responses: InterruptResponse[],
    idempotencyKey: string,
  ): Promise<void>;
}

export function InterruptSetCard(props: InterruptSetCardProps) {
  const { t, formatNumber } = useLocalization();
  const [drafts, setDrafts] = useState<Record<string, InterruptDraft>>(() =>
    createInterruptDrafts(props.interruptSet),
  );
  const [localError, setLocalError] = useState<string>();
  const intent = useRef<{ fingerprint: string; key: string } | undefined>(
    undefined,
  );
  const composing = useRef(false);

  const updateDraft = (
    itemId: string,
    update: (draft: InterruptDraft) => InterruptDraft,
  ) => {
    setDrafts((current) => ({
      ...current,
      [itemId]: update(current[itemId] ?? {}),
    }));
    setLocalError(undefined);
  };

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (props.pending || composing.current) return;
    try {
      const responses = buildInterruptResponses(props.interruptSet, drafts);
      const fingerprint = JSON.stringify(responses);
      const resumeIntent =
        intent.current?.fingerprint === fingerprint
          ? intent.current
          : { fingerprint, key: crypto.randomUUID() };
      intent.current = resumeIntent;
      setLocalError(undefined);
      await props.onResume(props.interruptSet, responses, resumeIntent.key);
      intent.current = undefined;
    } catch (error) {
      setLocalError(messageOf(error, t("interrupt.resumeFailed"), t));
    }
  };

  return (
    <form
      className="interrupt-set"
      aria-labelledby={`interrupt-title-${props.interruptSet.rootRunId}`}
      onSubmit={submit}
      onCompositionStartCapture={() => {
        composing.current = true;
      }}
      onCompositionEndCapture={() => {
        composing.current = false;
      }}
    >
      <header className="interrupt-heading">
        <div>
          <span className="eyebrow">{t("interrupt.actionRequired")}</span>
          <h3 id={`interrupt-title-${props.interruptSet.rootRunId}`}>
            {t("interrupt.waitingForYou")}
          </h3>
        </div>
        <span className="interrupt-count">
          {t(
            props.interruptSet.interrupts.length === 1
              ? "interrupt.requestCountOne"
              : "interrupt.requestCountMany",
            { count: formatNumber(props.interruptSet.interrupts.length) },
          )}
        </span>
      </header>
      <p className="interrupt-intro">{t("interrupt.intro")}</p>
      <div className="interrupt-requests">
        {props.interruptSet.interrupts.map((interrupt, index) => (
          <InterruptEditor
            key={interrupt.itemId}
            interrupt={interrupt}
            index={index}
            draft={drafts[interrupt.itemId] ?? {}}
            disabled={props.pending}
            onChange={(update) => updateDraft(interrupt.itemId, update)}
          />
        ))}
      </div>
      <footer className="interrupt-actions">
        <span>{t("interrupt.atomicNote")}</span>
        <button type="submit" disabled={props.pending}>
          {props.pending ? t("interrupt.continuing") : t("interrupt.submitAll")}
        </button>
      </footer>
      {localError || props.error ? (
        <p className="interrupt-error" role="alert">
          {localError ?? props.error}
        </p>
      ) : null}
    </form>
  );
}

interface InterruptEditorProps {
  interrupt: Interrupt;
  index: number;
  draft: InterruptDraft;
  disabled: boolean;
  onChange(update: (draft: InterruptDraft) => InterruptDraft): void;
}

function InterruptEditor(props: InterruptEditorProps) {
  const { t } = useLocalization();
  if (props.interrupt.type === "approval") {
    return <ApprovalInterrupt {...props} />;
  }
  if (props.interrupt.type === "question") {
    return <QuestionInterrupt {...props} />;
  }
  return (
    <section className="interrupt-request">
      <p role="alert">{t("interrupt.unsupported")}</p>
    </section>
  );
}

function messageOf(error: unknown, fallback: string, t: Translate) {
  if (error instanceof InterruptResponseValidationError) {
    switch (error.code) {
      case "unsupportedInteraction":
        return t("interrupt.unsupported");
      case "approvalDecisionRequired":
        return t("approval.decisionRequired");
      case "questionIncomplete":
        return t("question.incomplete");
      case "textAnswerRequired":
        return t("question.answerBeforeContinue", {
          prompt: error.detail ?? "",
        });
      case "unsupportedQuestionField":
        return t("question.unsupportedField", { type: error.detail ?? "" });
      case "choiceRequired":
        return t("question.chooseBeforeContinue", {
          prompt: error.detail ?? "",
        });
      case "singleChoiceRequired":
        return t("question.chooseOne", { prompt: error.detail ?? "" });
      case "argumentsInvalidJSON":
        return t("approval.argumentsInvalidJSON");
      case "argumentsNotObject":
        return t("approval.argumentsNotObject");
    }
  }
  return error instanceof Error ? error.message : fallback;
}
