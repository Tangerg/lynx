// Message content-block plugin surface.
//
// This file owns the discriminated union that plugin authors can extend via
// declaration merging. Keep it separate from agentView.ts: content blocks are
// an SDK extension contract, while AgentViewState is one built-in context's
// read model.

export type BlockStatus = "running" | "complete" | "incomplete" | "requires-action";

export interface QuestionOption {
  label: string;
  description: string;
  preview?: string;
}

// One clarifying field projected from a runtime question. The card renders
// these as single/multi-select choices with an optional free-text fallback.
export interface QuestionItem {
  id: string;
  question: string;
  header: string;
  options: QuestionOption[];
  multiSelect: boolean;
  allowFreeText?: boolean;
}

export interface BuiltinContentBlockMap {
  text: { kind: "text"; text: string; status: BlockStatus; itemId?: string };
  image: { kind: "image"; mime: string; data: string };
  reasoning: { kind: "reasoning"; reasoningId: string; text: string; status: BlockStatus };
  plan: { kind: "plan" };
  tool: { kind: "tool"; toolCallId: string };
  approval: {
    kind: "approval";
    status: BlockStatus;
    text: string;
    command: string;
    reason: string;
    itemId?: string;
    parentRunId?: string;
    decision?: "approved" | "declined";
    args?: Record<string, unknown>;
    risk?: "low" | "medium" | "high";
    scope?: string[];
    target?: string;
    reversible?: boolean;
  };
  question: {
    kind: "question";
    status: BlockStatus;
    itemId?: string;
    parentRunId?: string;
    questions: QuestionItem[];
    answered?: boolean;
    answers?: Record<string, string[]>;
  };
  compaction: { kind: "compaction"; summary?: string; droppedMessages?: number };
}

// Empty by design. Plugins augment this interface:
//
//   declare module "@/plugins/sdk/types/contentBlock" {
//     interface CustomContentBlockMap {
//       cpuChart: { kind: "cpuChart"; series: ChartPoint[] };
//     }
//   }
export interface CustomContentBlockMap {}

export type ContentBlockMap = BuiltinContentBlockMap & CustomContentBlockMap;
export type ContentBlockKind = keyof ContentBlockMap;
export type ContentBlock = ContentBlockMap[ContentBlockKind];
