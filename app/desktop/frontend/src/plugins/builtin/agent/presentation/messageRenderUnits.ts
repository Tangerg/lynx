import type { ContentBlock, ToolCall } from "@/plugins/builtin/agent/public/viewState";
import { isQuestionTool } from "@/plugins/builtin/agent/public/viewState";
import { isReadOnlyTool } from "./toolPresentation";

export type MessageRenderUnit =
  { kind: "block"; block: ContentBlock; index: number } | { kind: "toolGroup"; tools: ToolCall[] };

export function planRenderUnits(
  blocks: ContentBlock[],
  toolCalls: Record<string, ToolCall>,
): MessageRenderUnit[] {
  const units: MessageRenderUnit[] = [];
  const hasQuestion = blocks.some((block) => block.kind === "question");
  let run: { block: ContentBlock; index: number; tool: ToolCall }[] = [];

  const flush = () => {
    if (run.length >= 2) {
      units.push({ kind: "toolGroup", tools: run.map((item) => item.tool) });
    } else {
      for (const item of run) units.push({ kind: "block", block: item.block, index: item.index });
    }
    run = [];
  };

  blocks.forEach((block, index) => {
    if (block.kind === "tool") {
      const tool = toolCalls[block.toolCallId];
      if (tool && isReadOnlyTool(tool.name)) {
        run.push({ block, index, tool });
        return;
      }
      if (tool && hasQuestion && isQuestionTool(tool.name)) {
        flush();
        return;
      }
    }

    flush();
    units.push({ kind: "block", block, index });
  });

  flush();
  return units;
}
