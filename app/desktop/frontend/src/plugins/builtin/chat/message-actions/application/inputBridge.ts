import type { AgentInput } from "@/plugins/builtin/agent/public/input";
import type { ComposerDraftInput } from "../../composer/public/draft";
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

export function agentInputToComposerDraft(input: AgentInput): ComposerDraftInput {
  return {
    text: input.parts
      .filter(
        (part): part is Extract<AgentInput["parts"][number], { kind: "text" }> =>
          part.kind === "text",
      )
      .map((part) => part.text)
      .join("\n\n"),
    images: input.parts
      .filter(
        (part): part is Extract<AgentInput["parts"][number], { kind: "image" }> =>
          part.kind === "image",
      )
      .map((part) => ({ mime: part.mime, data: part.data })),
  };
}
