import { useCallback } from "react";
import { useChatSend, type AgentInput } from "@/plugins/builtin/agent/public/input";
import type { UserInput } from "../public/input";

function composerInputToAgentInput(input: UserInput): AgentInput {
  return {
    parts: input.parts.map((part) =>
      part.kind === "text"
        ? { kind: "text", text: part.text }
        : { kind: "image", mime: part.mime, data: part.data },
    ),
  };
}

export function useSendComposerInput(): (input: UserInput) => void {
  const sendAgentInput = useChatSend();
  return useCallback(
    (input: UserInput) => {
      sendAgentInput(composerInputToAgentInput(input));
    },
    [sendAgentInput],
  );
}
