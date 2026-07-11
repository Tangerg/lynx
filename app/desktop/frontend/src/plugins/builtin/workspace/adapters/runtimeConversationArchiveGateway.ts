import { getContainer } from "@/main/container";
import { asSessionId, type SessionArtifact } from "@/rpc";
import { configureConversationArchiveGateway } from "../application/ports/conversationArchiveGateway";
import type { ConversationArchiveGateway } from "../application/ports/conversationArchiveGateway";

const gateway: ConversationArchiveGateway = {
  async exportConversation(sessionId, format) {
    return getContainer().client().sessions.export(asSessionId(sessionId), format);
  },
  async importConversation(artifact) {
    const { session } = await getContainer()
      .client()
      .sessions.import(artifact as SessionArtifact);
    return {
      id: session.id,
      title: session.title,
    };
  },
};

export function installConversationArchiveGateway(): () => void {
  return configureConversationArchiveGateway(gateway);
}
