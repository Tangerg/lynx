import type { MessageFeedbackRating } from "../domain/feedback";

export interface MessageFeedbackTarget {
  messageId: string;
  runId?: string;
}

export interface SubmitMessageFeedbackPort {
  createMessageFeedback(input: {
    target: MessageFeedbackTarget;
    rating: MessageFeedbackRating;
  }): Promise<void>;
}

const ratedMessages = new Map<string, MessageFeedbackRating>();

export function messageFeedbackRating(messageId: string): MessageFeedbackRating | undefined {
  return ratedMessages.get(messageId);
}

export async function submitMessageFeedback(
  target: MessageFeedbackTarget,
  rating: MessageFeedbackRating,
  port: SubmitMessageFeedbackPort,
): Promise<MessageFeedbackRating> {
  const previous = ratedMessages.get(target.messageId);
  if (previous === rating) return rating;
  ratedMessages.set(target.messageId, rating);
  try {
    await port.createMessageFeedback({ target, rating });
    return rating;
  } catch (error) {
    if (previous) ratedMessages.set(target.messageId, previous);
    else ratedMessages.delete(target.messageId);
    throw error;
  }
}
