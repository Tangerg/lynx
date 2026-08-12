import type { ContentBlock } from "@/rpc";
import type { AgentInput } from "../domain/input";

export function agentInputToContentBlocks(input: AgentInput): ContentBlock[] {
  return input.parts.map((part) =>
    part.kind === "text"
      ? { type: "text", text: part.text }
      : { type: "image", mime: part.mime, data: part.data },
  );
}

export function contentBlocksToAgentInput(blocks: readonly ContentBlock[]): AgentInput {
  return {
    parts: blocks.map((block) =>
      block.type === "text"
        ? { kind: "text", text: block.text }
        : { kind: "image", mime: block.mime, data: block.data },
    ),
  };
}
