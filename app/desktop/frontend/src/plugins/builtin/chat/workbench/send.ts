import { useCallback } from "react";
import { useChatSend } from "@/plugins/builtin/agent/public/input";
import type { UserInput } from "../composer/public/input";
import { composerInputToAgentInput } from "./inputBridge";

export function useSendComposerInput(): (input: UserInput) => void {
  const sendAgentInput = useChatSend();
  return useCallback(
    (input: UserInput) => {
      sendAgentInput(composerInputToAgentInput(input));
    },
    [sendAgentInput],
  );
}
