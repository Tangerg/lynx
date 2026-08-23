import type { KeyboardEvent } from "react";

import type {
  Interrupt,
  QuestionField,
} from "@lyra/runtime-contract";

import {
  createQuestionDraft,
  type InterruptDraft,
} from "./interruptResponse";

interface QuestionInterruptProps {
  interrupt: Interrupt;
  index: number;
  draft: InterruptDraft;
  disabled: boolean;
  onChange(update: (draft: InterruptDraft) => InterruptDraft): void;
}

export function QuestionInterrupt(props: QuestionInterruptProps) {
  const question = props.interrupt.payload?.question;
  const draft = props.draft.question ?? createQuestionDraft(props.interrupt);
  const updateField = (
    fieldIndex: number,
    values: string[],
    custom = draft.custom[fieldIndex] ?? "",
  ) => {
    props.onChange((current) => {
      const source = current.question ?? draft;
      const nextValues = source.values.map((value) => [...value]);
      const nextCustom = [...source.custom];
      nextValues[fieldIndex] = values;
      nextCustom[fieldIndex] = custom;
      return {
        ...current,
        question: { values: nextValues, custom: nextCustom },
      };
    });
  };

  return (
    <section className="interrupt-request question-request">
      <div className="request-title">
        <span>{props.index + 1}</span>
        <div>
          <small>Question</small>
          <h4>Input for Lyra</h4>
        </div>
      </div>
      {question ? (
        <div className="question-fields">
          {question.fields.map((field, fieldIndex) => (
            <QuestionInput
              key={`${props.interrupt.itemId}:${fieldIndex}`}
              itemId={props.interrupt.itemId}
              field={field}
              fieldIndex={fieldIndex}
              values={draft.values[fieldIndex] ?? []}
              custom={draft.custom[fieldIndex] ?? ""}
              disabled={props.disabled}
              onChange={(values, custom) =>
                updateField(fieldIndex, values, custom)
              }
            />
          ))}
        </div>
      ) : (
        <p role="alert">The Runtime did not provide the question fields.</p>
      )}
    </section>
  );
}

interface QuestionInputProps {
  itemId: string;
  field: QuestionField;
  fieldIndex: number;
  values: string[];
  custom: string;
  disabled: boolean;
  onChange(values: string[], custom: string): void;
}

function QuestionInput(props: QuestionInputProps) {
  const submitOnShortcut = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (
      event.key === "Enter" &&
      (event.metaKey || event.ctrlKey) &&
      !event.nativeEvent.isComposing
    ) {
      event.preventDefault();
      event.currentTarget.form?.requestSubmit();
    }
  };
  return (
    <fieldset className="question-field">
      <legend>
        {props.field.header ? <span>{props.field.header}</span> : null}
        {props.field.prompt}
      </legend>
      {props.field.type === "text" ? (
        <textarea
          value={props.values[0] ?? ""}
          rows={3}
          disabled={props.disabled}
          placeholder="Type your answer"
          onKeyDown={submitOnShortcut}
          onChange={(event) => props.onChange([event.currentTarget.value], "")}
        />
      ) : props.field.type === "choice" ? (
        <ChoiceInput {...props} />
      ) : (
        <p role="alert">Unsupported question field type: {props.field.type}</p>
      )}
    </fieldset>
  );
}

function ChoiceInput(props: QuestionInputProps) {
  return (
    <>
      <div className="question-options">
        {(props.field.options ?? []).map((option, optionIndex) => {
          const checked = props.values.includes(option.label);
          return (
            <label
              key={`${option.label}:${optionIndex}`}
              data-selected={checked}
            >
              <input
                type={props.field.multiple ? "checkbox" : "radio"}
                name={`${props.itemId}:${props.fieldIndex}`}
                value={option.label}
                checked={checked}
                disabled={props.disabled}
                onChange={() => {
                  if (props.field.multiple) {
                    props.onChange(
                      checked
                        ? props.values.filter(
                            (value) => value !== option.label,
                          )
                        : [...props.values, option.label],
                      props.custom,
                    );
                  } else {
                    props.onChange([option.label], "");
                  }
                }}
              />
              <span>
                <strong>{option.label}</strong>
                {option.description ? <small>{option.description}</small> : null}
                {option.preview ? <code>{option.preview}</code> : null}
              </span>
            </label>
          );
        })}
      </div>
      {props.field.allowCustom ? (
        <label className="custom-answer">
          <span>Custom answer</span>
          <input
            value={props.custom}
            disabled={props.disabled}
            placeholder="Write another answer"
            onChange={(event) =>
              props.onChange(
                props.field.multiple ? props.values : [],
                event.currentTarget.value,
              )
            }
          />
        </label>
      ) : null}
    </>
  );
}
