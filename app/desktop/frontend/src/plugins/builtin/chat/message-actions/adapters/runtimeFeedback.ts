import { getContainer } from "@/main/container";
import type { LyraClient } from "@/rpc";
import { asItemId, asRunId, asSessionId } from "@/rpc";
import { MessageFeedbackOwner, type MessageFeedbackGateway } from "../application/feedback";

function runtimeFeedbackGateway(client: LyraClient): MessageFeedbackGateway {
  return {
    async createMessageFeedback({ target, rating }) {
      await client.feedback.create({
        sessionId: asSessionId(target.sessionId),
        runId: target.runId ? asRunId(target.runId) : undefined,
        itemId: asItemId(target.messageId),
        rating,
      });
    },
  };
}

interface RuntimeFeedbackInstallation {
  replaceRuntimeGeneration(): void;
  dispose(): void;
}

export function installRuntimeFeedbackGateway(): RuntimeFeedbackInstallation {
  const owner = MessageFeedbackOwner.install(runtimeFeedbackGateway(getContainer().client()));
  return {
    replaceRuntimeGeneration: () => owner.replaceRuntimeGeneration(),
    dispose: () => owner.dispose(),
  };
}
