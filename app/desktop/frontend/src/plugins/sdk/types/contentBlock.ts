// Message content blocks projected by the Runtime fold.

export type BlockStatus = "running" | "complete" | "incomplete" | "requires-action";

export interface QuestionOption {
  label: string;
  description: string;
  preview?: string;
}

interface QuestionItemBase {
  prompt: string;
  header: string;
}

export interface TextQuestionItem extends QuestionItemBase {
  type: "text";
}

export interface ChoiceQuestionItem extends QuestionItemBase {
  type: "choice";
  options: QuestionOption[];
  multiple: boolean;
  allowCustom: boolean;
}

// One required clarifying field projected from the runtime's closed union.
export type QuestionItem = TextQuestionItem | ChoiceQuestionItem;

interface ContentBlockMap {
  text: { kind: "text"; text: string; status: BlockStatus; itemId?: string };
  image: { kind: "image"; mime: string; data: string };
  reasoning: { kind: "reasoning"; reasoningId: string; text: string; status: BlockStatus };
  tool: { kind: "tool"; toolCallId: string };
  approval: {
    kind: "approval";
    status: BlockStatus;
    /** The tool whose call needs a decision. The card localizes its headline at
     *  render. Absent when the runtime named no tool. */
    toolName?: string;
    command: string;
    reason: string;
    itemId?: string;
    runId?: string;
    decision?: "approved" | "declined";
    args?: Record<string, unknown>;
    rememberable?: boolean;
  };
  question: {
    kind: "question";
    status: BlockStatus;
    itemId?: string;
    runId?: string;
    questions: QuestionItem[];
    answered?: boolean;
    answers?: string[][];
  };
  compaction: { kind: "compaction"; summary?: string; droppedMessages?: number };
}

export type ContentBlock = ContentBlockMap[keyof ContentBlockMap];
