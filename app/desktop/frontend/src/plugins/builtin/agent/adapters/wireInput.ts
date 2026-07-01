import type { ContentBlock } from "@/rpc";
import type { AgentInput } from "../domain/input";

export function agentInputToContentBlocks(input: AgentInput): ContentBlock[] {
  return input.parts.map((part) =>
    part.kind === "text"
      ? { type: "text", text: part.text }
      : { type: "image", mime: part.mime, data: part.data },
  );
}
