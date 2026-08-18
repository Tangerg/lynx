import { useT } from "@/lib/i18n";
import {
  contributeLayout,
  definePlugin,
  useCurrentMessage,
  useCurrentMessageSessionId,
} from "@/plugins/sdk";
import { RUNTIME_STREAM_PORTS } from "@/plugins/builtin/runtime/public/ports";
import type { Message } from "@/plugins/builtin/agent/public/viewState";
import type { MessageFeedbackRating } from "./domain/feedback";
import { canRateMessage } from "./application/messageActionAvailability";
import { messageFeedbackWasRetired, useMessageFeedback } from "./public/feedback";
import { installRuntimeFeedbackGateway } from "./adapters/runtimeFeedback";
import { MessageActionButton } from "./MessageActionButton";

function FeedbackButtons() {
  const msg = useCurrentMessage();
  if (!canRateMessage(msg)) return null;
  return <RateableFeedbackButtons msg={msg} />;
}

function RateableFeedbackButtons({ msg }: { msg: Message }) {
  const t = useT();
  const sessionId = useCurrentMessageSessionId();
  const feedback = useMessageFeedback(sessionId, msg);

  const rate = (rating: MessageFeedbackRating): void => {
    if (feedback.rating === rating) return;
    void feedback.submit(rating).catch((error: unknown) => {
      if (!messageFeedbackWasRetired(error)) console.warn("[feedback] create failed:", error);
    });
  };

  return (
    <>
      <MessageActionButton
        icon="thumbs-up"
        title={t("msgActions.good")}
        role={msg.role}
        aria-pressed={feedback.rating === "positive"}
        onClick={() => rate("positive")}
        className={feedback.rating === "positive" ? "text-success" : undefined}
      />
      <MessageActionButton
        icon="thumbs-down"
        title={t("msgActions.poor")}
        role={msg.role}
        aria-pressed={feedback.rating === "negative"}
        onClick={() => rate("negative")}
        className={feedback.rating === "negative" ? "text-negative" : undefined}
      />
    </>
  );
}

export const messageFeedback = definePlugin({
  name: "lyra.builtin.message-feedback",
  requires: { runtime: RUNTIME_STREAM_PORTS },
  setup(ctx) {
    const gateway = installRuntimeFeedbackGateway();
    let connectionGeneration = ctx.runtime.connectionGeneration();
    const unsubscribeRuntime = ctx.runtime.subscribeConnection(() => {
      const next = ctx.runtime.connectionGeneration();
      if (next === connectionGeneration) return;
      connectionGeneration = next;
      gateway.replaceRuntimeGeneration();
    });
    contributeLayout(ctx, "message.actions", {
      id: "feedback",
      order: 15,
      component: FeedbackButtons,
    });
    ctx.cleanup(() => {
      unsubscribeRuntime();
      gateway.dispose();
    });
  },
});
