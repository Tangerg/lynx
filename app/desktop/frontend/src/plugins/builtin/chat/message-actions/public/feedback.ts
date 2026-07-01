import type { Message } from "@/plugins/builtin/agent/public/viewState";
import {
  messageFeedbackRating,
  submitMessageFeedback as submitMessageFeedbackIntent,
} from "../application/feedback";
import type { MessageFeedbackRating } from "../domain/feedback";

export { messageFeedbackRating };
export type { MessageFeedbackRating } from "../domain/feedback";

export async function submitMessageFeedback(
  msg: Message,
  rating: MessageFeedbackRating,
): Promise<MessageFeedbackRating> {
  try {
    return await submitMessageFeedbackIntent({ messageId: msg.id, runId: msg.runId }, rating);
  } catch (error) {
    console.warn("[feedback] create failed:", error);
    throw error;
  }
}
