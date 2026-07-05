// Edit action (user messages only) — load the text back into the composer so
// the user can tweak and re-send. Doesn't mutate the original message; sending
// creates a new user turn.

import { Icon, Tooltip } from "@/ui";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import { definePlugin, useCurrentMessage } from "@/plugins/sdk";
import { messageEditActionSlot } from "./application/messageActionContributions";
import { messageHasDraftContent } from "./application/messageActionContent";
import { editMessageInComposer } from "./public/messageActions";
import { ACTION_BTN_BASE, roleShape } from "./_shared";

function EditButton() {
  const t = useT();
  const msg = useCurrentMessage();
  if (msg.role !== "user") return null;
  if (!messageHasDraftContent(msg)) return null;

  return (
    <Tooltip label={t("msgActions.edit")}>
      <button
        type="button"
        onClick={() => editMessageInComposer(msg)}
        aria-label={t("msgActions.edit")}
        className={cn(ACTION_BTN_BASE, roleShape(msg.role))}
      >
        <Icon name="edit" size={13} />
      </button>
    </Tooltip>
  );
}

export const messageEdit = definePlugin({
  name: "lyra.builtin.message-edit",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("message.actions", messageEditActionSlot(EditButton));
  },
});
