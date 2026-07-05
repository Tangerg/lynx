// Regenerate action (assistant messages only) — replay the preceding user
// prompt via the shared regenerate message action.

import { Icon, Tooltip } from "@/ui";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import { definePlugin, useCurrentMessage } from "@/plugins/sdk";
import { canRegenerateMessage } from "./application/messageActionAvailability";
import { messageRegenerateActionSlot } from "./application/messageActionContributions";
import { regenerateMessage } from "./public/messageActions";
import { ACTION_BTN_BASE, roleShape } from "./_shared";

function RegenerateButton() {
  const t = useT();
  const msg = useCurrentMessage();
  if (!canRegenerateMessage(msg)) return null;

  return (
    <Tooltip label={t("msgActions.regenerate")}>
      <button
        type="button"
        onClick={() => regenerateMessage(msg)}
        aria-label={t("msgActions.regenerate")}
        className={cn(ACTION_BTN_BASE, roleShape(msg.role))}
      >
        <Icon name="loop" size={13} />
      </button>
    </Tooltip>
  );
}

export const messageRegenerate = definePlugin({
  name: "lyra.builtin.message-regenerate",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("message.actions", messageRegenerateActionSlot(RegenerateButton));
  },
});
