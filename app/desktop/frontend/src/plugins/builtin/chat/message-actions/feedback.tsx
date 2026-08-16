// Feedback action (assistant messages only) — thumbs up / down wired to
// `feedback.create`. The wire is write-only (no read-back API), so the settled
// rating lives in a session-lifetime map — same scope as the approval "remember"
// decisions. Re-rating re-submits; the runtime treats each as a new event.

import { useEffect, useState } from "react";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { contributeLayout, definePlugin, useCurrentMessage } from "@/plugins/sdk";
import type { MessageFeedbackRating } from "./domain/feedback";
import { canRateMessage } from "./application/messageActionAvailability";
import { messageFeedbackActionSlot } from "./application/messageActionContributions";
import { messageFeedbackRating, submitMessageFeedback } from "./public/feedback";
import { installRuntimeFeedbackPort } from "./adapters/runtimeFeedback";
import { MessageActionButton } from "./MessageActionButton";

function FeedbackButtons() {
  const t = useT();
  const msg = useCurrentMessage();
  const [rated, setRated] = useState(() => messageFeedbackRating(msg.id));
  useEffect(() => {
    setRated(messageFeedbackRating(msg.id));
  }, [msg.id]);
  if (!canRateMessage(msg)) return null;

  const rate = (rating: MessageFeedbackRating): void => {
    if (rated === rating) return;
    setRated(rating);
    void submitMessageFeedback(msg, rating).catch(() => setRated(messageFeedbackRating(msg.id)));
  };

  return (
    <>
      <MessageActionButton
        icon="thumbs-up"
        title={t("msgActions.good")}
        role={msg.role}
        aria-pressed={rated === "positive"}
        onClick={() => rate("positive")}
        className={cn(rated === "positive" && "text-success")}
      />
      <MessageActionButton
        icon="thumbs-down"
        title={t("msgActions.poor")}
        role={msg.role}
        aria-pressed={rated === "negative"}
        onClick={() => rate("negative")}
        className={cn(rated === "negative" && "text-negative")}
      />
    </>
  );
}

export const messageFeedback = definePlugin({
  name: "lyra.builtin.message-feedback",
  setup(ctx) {
    const disposeFeedback = installRuntimeFeedbackPort();
    contributeLayout(ctx, "message.actions", messageFeedbackActionSlot(FeedbackButtons));
    ctx.cleanup(disposeFeedback);
  },
});
