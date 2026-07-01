import type { AgentInput } from "@/plugins/builtin/agent/public/input";
import type { UserInput } from "../../composer/public/input";

export function composerInputToAgentInput(input: UserInput): AgentInput {
  return {
    parts: input.parts.map((part) =>
      part.kind === "text"
        ? { kind: "text", text: part.text }
        : { kind: "image", mime: part.mime, data: part.data },
    ),
  };
}
