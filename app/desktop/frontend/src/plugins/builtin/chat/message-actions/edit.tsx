// Edit action (user messages only) — load the text back into the composer so
// the user can tweak and re-send. Doesn't mutate the original message; sending
// creates a new user turn.

import { useT } from "@/lib/i18n";
import { contributeLayout, definePlugin, useCurrentMessage } from "@/plugins/sdk";
import { canEditMessage } from "./application/messageActionAvailability";
import { messageEditActionSlot } from "./application/messageActionContributions";
import { editMessageInComposer } from "./public/messageActions";
import { MessageActionButton } from "./MessageActionButton";

function EditButton() {
  const t = useT();
  const msg = useCurrentMessage();
  if (!canEditMessage(msg)) return null;

  return (
    <MessageActionButton
      icon="edit"
      title={t("msgActions.edit")}
      role={msg.role}
      onClick={() => editMessageInComposer(msg)}
    />
  );
}

export const messageEdit = definePlugin({
  name: "lyra.builtin.message-edit",
  setup(ctx) {
    contributeLayout(ctx, "message.actions", messageEditActionSlot(EditButton));
  },
});
