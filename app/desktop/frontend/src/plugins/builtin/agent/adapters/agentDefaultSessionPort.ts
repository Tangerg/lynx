import { pickAgentSource } from "@/plugins/sdk";
import { navigator } from "@/lib/navigation";
import { configureAgentDefaultSessionPort } from "../application/ports/defaultSession";
import { useAgentSession } from "./useAgentSession";

export function installAgentDefaultSessionPort(): () => void {
  return configureAgentDefaultSessionPort({
    useDefaultChatSession,
  });
}

function useDefaultChatSession() {
  const activeSessionId = navigator().use((location) => location.session);
  return useAgentSession(() => {
    const source = pickAgentSource();
    if (!source) throw new Error("No agent source registered");
    return source.factory();
  }, activeSessionId);
}
