// Regenerate action (assistant messages only) — replay the preceding user
// prompt via the shared regenerate message action.

import { useT } from "@/lib/i18n";
import { definePlugin, useCurrentMessage } from "@/plugins/sdk";
import { canRegenerateMessage } from "./application/messageActionAvailability";
import { messageRegenerateActionSlot } from "./application/messageActionContributions";
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
  name: "lyra.builtin.message-regenerate",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("message.actions", messageRegenerateActionSlot(RegenerateButton));
  },
});
