import type {
  Interrupt,
  InterruptResponse,
  InterruptResponseValue,
  PendingInterruptSet,
} from "@lyra/runtime-contract";

export interface ApprovalDraft {
  decision?: "approve" | "deny";
  argumentsText: string;
  reason: string;
  remember: "once" | "session" | "project" | "global";
}

export interface QuestionDraft {
  values: string[][];
  custom: string[];
}

export interface InterruptDraft {
  approval?: ApprovalDraft;
  question?: QuestionDraft;
}

export function createInterruptDrafts(
  interruptSet: PendingInterruptSet,
): Record<string, InterruptDraft> {
  return Object.fromEntries(
    interruptSet.interrupts.map((interrupt) => [
      interrupt.itemId,
      {
        ...(interrupt.type === "approval"
          ? { approval: createApprovalDraft(interrupt) }
          : {}),
        ...(interrupt.type === "question"
          ? { question: createQuestionDraft(interrupt) }
          : {}),
      },
    ]),
  );
}

export function createApprovalDraft(interrupt: Interrupt): ApprovalDraft {
  return {
    argumentsText: JSON.stringify(
      interrupt.payload?.tool?.arguments ?? {},
      null,
      2,
    ),
    reason: "",
    remember: "once",
  };
}

export function createQuestionDraft(interrupt: Interrupt): QuestionDraft {
  const fields = interrupt.payload?.question?.fields ?? [];
  return {
    values: fields.map(() => []),
    custom: fields.map(() => ""),
  };
}

export function buildInterruptResponses(
  interruptSet: PendingInterruptSet,
  drafts: Record<string, InterruptDraft>,
): InterruptResponse[] {
  return interruptSet.interrupts.map((interrupt) => {
    const draft = drafts[interrupt.itemId];
    if (interrupt.type === "approval") {
      return {
        itemId: interrupt.itemId,
        response: approvalResponse(interrupt, draft?.approval),
      };
    }
    if (interrupt.type === "question") {
      return {
        itemId: interrupt.itemId,
        response: questionResponse(interrupt, draft?.question),
      };
    }
    throw new Error("This Runtime requested an unsupported interaction.");
  });
}

function approvalResponse(
  interrupt: Interrupt,
  draft: ApprovalDraft | undefined,
): InterruptResponseValue {
  if (draft?.decision === undefined) {
    throw new Error("Choose Approve or Deny for every approval request.");
  }
  const original = interrupt.payload?.tool?.arguments ?? {};
  const edited = parseArguments(draft.argumentsText);
  const changed = JSON.stringify(edited) !== JSON.stringify(original);
  const reason = draft.reason.trim();
  return {
    type: "approval",
    decision: draft.decision,
    ...(interrupt.payload?.rememberable && draft.remember !== "once"
      ? { remember: { scope: draft.remember } }
      : {}),
    ...(changed ? { editedArgs: edited } : {}),
    ...(reason === "" ? {} : { reason }),
  };
}

function questionResponse(
  interrupt: Interrupt,
  draft: QuestionDraft | undefined,
): InterruptResponseValue {
  const question = interrupt.payload?.question;
  if (question === undefined || draft === undefined) {
    throw new Error("The Runtime did not provide a complete question.");
  }
  const answers = question.fields.map((field, index) => {
    if (field.type === "text") {
      const answer = draft.values[index]?.[0]?.trim() ?? "";
      if (answer === "") {
        throw new Error(`Answer “${field.prompt}” before continuing.`);
      }
      return [answer];
    }
    if (field.type !== "choice") {
      throw new Error(`Unsupported question field type: ${field.type}`);
    }
    const selected = (draft.values[index] ?? [])
      .map((value) => value.trim())
      .filter(Boolean);
    const custom = draft.custom[index]?.trim() ?? "";
    const values =
      custom === "" || selected.includes(custom)
        ? selected
        : [...selected, custom];
    if (values.length === 0) {
      throw new Error(
        `Choose an answer for “${field.prompt}” before continuing.`,
      );
    }
    if (!field.multiple && values.length !== 1) {
      throw new Error(`Choose one answer for “${field.prompt}”.`);
    }
    return values;
  });
  return { type: "answer", answers };
}

function parseArguments(value: string): Record<string, unknown> {
  let parsed: unknown;
  try {
    parsed = JSON.parse(value);
  } catch {
    throw new Error("Edited tool arguments must be valid JSON.");
  }
  if (typeof parsed !== "object" || parsed === null || Array.isArray(parsed)) {
    throw new Error("Edited tool arguments must be a JSON object.");
  }
  return parsed as Record<string, unknown>;
}
