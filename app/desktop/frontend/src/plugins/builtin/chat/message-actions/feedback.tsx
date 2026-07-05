// Feedback action (assistant messages only) — thumbs up / down wired to
// `feedback.create`. The wire is write-only (no read-back API), so the settled
// rating lives in a session-lifetime map — same scope as the approval "remember"
// decisions. Re-rating re-submits; the runtime treats each as a new event.

import { useEffect, useState } from "react";
import { Icon, Tooltip } from "@/ui";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import { definePlugin, useCurrentMessage } from "@/plugins/sdk";
import { messageFeedbackActionSlot } from "./application/messageActionContributions";
import { messageFeedbackRating, submitMessageFeedback } from "./public/feedback";
import { installRuntimeFeedbackPort } from "./adapters/runtimeFeedback";
import { ACTION_BTN_BASE, roleShape } from "./_shared";

function FeedbackButtons() {
  const t = useT();
  const msg = useCurrentMessage();
  const [rated, setRated] = useState(() => messageFeedbackRating(msg.id));
  useEffect(() => {
    setRated(messageFeedbackRating(msg.id));
  }, [msg.id]);
  if (msg.role !== "assistant") return null;

  const rate = (rating: "positive" | "negative"): void => {
    if (rated === rating) return;
    setRated(rating);
    void submitMessageFeedback(msg, rating).catch(() => setRated(messageFeedbackRating(msg.id)));
  };

  return (
    <>
      <Tooltip label={t("msgActions.good")}>
        <button
          type="button"
          onClick={() => rate("positive")}
          aria-label={t("msgActions.good")}
          aria-pressed={rated === "positive"}
          className={cn(
            ACTION_BTN_BASE,
            roleShape(msg.role),
            rated === "positive" && "text-success",
          )}
        >
          <Icon name="thumbs-up" size={13} />
        </button>
      </Tooltip>
      <Tooltip label={t("msgActions.poor")}>
        <button
          type="button"
          onClick={() => rate("negative")}
          aria-label={t("msgActions.poor")}
          aria-pressed={rated === "negative"}
          className={cn(
            ACTION_BTN_BASE,
            roleShape(msg.role),
            rated === "negative" && "text-negative",
          )}
        >
          <Icon name="thumbs-down" size={13} />
        </button>
      </Tooltip>
    </>
  );
}

export const messageFeedback = definePlugin({
  name: "lyra.builtin.message-feedback",
  version: "1.0.0",
  setup({ host }) {
    installRuntimeFeedbackPort();
    host.layout.register("message.actions", messageFeedbackActionSlot(FeedbackButtons));
  },
});
