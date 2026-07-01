import { getContainer } from "@/main/container";
import { asItemId, asRunId, asSessionId } from "@/rpc";
import { useSessionStore } from "@/state/sessionStore";
import type { SubmitMessageFeedbackPort } from "../application/feedback";

export const runtimeFeedbackPort: SubmitMessageFeedbackPort = {
  async createMessageFeedback({ target, rating }) {
    const sessionId = useSessionStore.getState().activeSessionId;
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
