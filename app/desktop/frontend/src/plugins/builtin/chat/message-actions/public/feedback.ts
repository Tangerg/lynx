import { useCallback, useMemo, useSyncExternalStore } from "react";
import type { Message } from "@/plugins/builtin/agent/public/viewState";
import {
  messageFeedbackRating,
  messageFeedbackWasRetired,
  submitMessageFeedback as submitMessageFeedbackIntent,
  subscribeMessageFeedback,
  type MessageFeedbackTarget,
} from "../application/feedback";
import type { MessageFeedbackRating } from "../domain/feedback";

export { messageFeedbackWasRetired };

interface MessageFeedbackModel {
  rating: MessageFeedbackRating | undefined;
  submit(rating: MessageFeedbackRating): Promise<MessageFeedbackRating>;
}

export function useMessageFeedback(sessionId: string, message: Message): MessageFeedbackModel {
  const target = useMemo<MessageFeedbackTarget>(
    () => ({
      sessionId,
      messageId: message.id,
      runId: message.runId ?? undefined,
    }),
    [message.id, message.runId, sessionId],
  );
  const subscribe = useCallback(
    (listener: () => void) => subscribeMessageFeedback(target, listener),
    [target],
  );
  const snapshot = useCallback(() => messageFeedbackRating(target), [target]);
  const rating = useSyncExternalStore(subscribe, snapshot, snapshot);
  const submit = useCallback(
    (next: MessageFeedbackRating) => submitMessageFeedbackIntent(target, next),
    [target],
  );
  return useMemo(() => ({ rating, submit }), [rating, submit]);
}
