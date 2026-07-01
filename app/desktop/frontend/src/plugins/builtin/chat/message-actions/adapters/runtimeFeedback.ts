import { getContainer } from "@/main/container";
import { getActiveSessionId } from "@/plugins/builtin/agent/public/session";
import { asItemId, asRunId, asSessionId } from "@/rpc";
import {
  configureMessageFeedbackPort,
  type SubmitMessageFeedbackPort,
} from "../application/feedback";

export const runtimeFeedbackPort: SubmitMessageFeedbackPort = {
  async createMessageFeedback({ target, rating }) {
    const sessionId = getActiveSessionId();
    await getContainer()
      .client()
      .feedback.create({
        sessionId: sessionId ? asSessionId(sessionId) : undefined,
        runId: target.runId ? asRunId(target.runId) : undefined,
        itemId: asItemId(target.messageId),
        rating,
      });
  },
};

export function installRuntimeFeedbackPort(): void {
  configureMessageFeedbackPort(runtimeFeedbackPort);
}
