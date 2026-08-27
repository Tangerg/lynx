import { getContainer } from "@/main/container";
import type { ScopeAppClient } from "@/rpc";
import { asItemId, asRunId, asSessionId } from "@/rpc";
import { MessageFeedbackOwner, type MessageFeedbackGateway } from "../application/feedback";

function runtimeFeedbackGateway(client: ScopeAppClient): MessageFeedbackGateway {
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

export function installRuntimeFeedbackGateway() {
  const owner = MessageFeedbackOwner.install(runtimeFeedbackGateway(getContainer().client()));
  return {
    replaceRuntimeGeneration: () => owner.replaceRuntimeGeneration(),
    dispose: () => owner.dispose(),
  };
}
