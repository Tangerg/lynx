import { getContainer } from "@/main/container";
import { asSessionId, type LyraClient, type SessionArtifact } from "@/rpc";
import { ConversationArchiveOwner } from "../application/conversationExport";
import type { ConversationArchiveGateway } from "../application/ports/conversationArchiveGateway";
import { browserFileTransfer } from "./browserFileTransfer";

function runtimeConversationArchiveGateway(client: LyraClient): ConversationArchiveGateway {
  return {
    async exportConversation(sessionId, format) {
      return client.sessions.export(asSessionId(sessionId), format);
    },
    async importConversation(artifact) {
      const { session } = await client.sessions.import(artifact as SessionArtifact);
      return {
        id: session.id,
        title: session.title,
      };
    },
  };
}

export function installConversationArchiveGateway() {
  const owner = ConversationArchiveOwner.install({
    gateway: runtimeConversationArchiveGateway(getContainer().client()),
    files: browserFileTransfer(),
  });
  return {
    replaceRuntimeGeneration: () => owner.replaceRuntimeGeneration(),
    dispose: () => owner.dispose(),
  };
}
