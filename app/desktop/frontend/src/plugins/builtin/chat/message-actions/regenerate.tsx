// Regenerate action (assistant messages only) — replay the preceding user
// prompt via the shared regenerate message action.

import { useT } from "@/lib/i18n";
import { contributeLayout, definePlugin, useCurrentMessage } from "@/plugins/sdk";
import { canRegenerateMessage } from "./application/messageActionAvailability";
import { regenerateMessage } from "./public/messageActions";
import { MessageActionButton } from "./MessageActionButton";

function RegenerateButton() {
  const t = useT();
  const msg = useCurrentMessage();
  if (!canRegenerateMessage(msg)) return null;

  return (
    <MessageActionButton
      icon="loop"
      title={t("msgActions.regenerate")}
      role={msg.role}
      onClick={() => regenerateMessage(msg)}
    />
  );
}

export const messageRegenerate = definePlugin({
  name: "scopeapp.builtin.message-regenerate",
  setup(ctx) {
    contributeLayout(ctx, "message.actions", {
      id: "regenerate",
      order: 10,
      component: RegenerateButton,
    });
  },
});
