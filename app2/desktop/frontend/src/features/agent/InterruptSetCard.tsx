import {
  useRef,
  useState,
  type FormEvent,
} from "react";

import type {
  Interrupt,
  InterruptResponse,
  PendingInterruptSet,
} from "@lyra/runtime-contract";

import { ApprovalInterrupt } from "./ApprovalInterrupt";
import { QuestionInterrupt } from "./QuestionInterrupt";
import {
  buildInterruptResponses,
  createInterruptDrafts,
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
      await props.onResume(
        props.interruptSet,
        responses,
        resumeIntent.key,
      );
      intent.current = undefined;
    } catch (error) {
      setLocalError(messageOf(error));
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
          <span className="eyebrow">Action required</span>
          <h3 id={`interrupt-title-${props.interruptSet.rootRunId}`}>
            Lyra is waiting for you
          </h3>
        </div>
        <span className="interrupt-count">
          {props.interruptSet.interrupts.length} request
          {props.interruptSet.interrupts.length === 1 ? "" : "s"}
        </span>
      </header>
      <p className="interrupt-intro">
        Review every request below. The complete set is committed together before
        this run continues.
      </p>
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
        <span>Answers are applied atomically to this waiting run.</span>
        <button type="submit" disabled={props.pending}>
          {props.pending ? "Continuing…" : "Submit all & continue"}
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
  if (props.interrupt.type === "approval") {
    return <ApprovalInterrupt {...props} />;
  }
  if (props.interrupt.type === "question") {
    return <QuestionInterrupt {...props} />;
  }
  return (
    <section className="interrupt-request">
      <p role="alert">This Runtime requested an unsupported interaction.</p>
    </section>
  );
}

function messageOf(error: unknown) {
  return error instanceof Error ? error.message : "The run could not be resumed.";
}
