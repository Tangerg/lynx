import { t } from "@/lib/i18n";
import { definePlugin } from "@/plugins/sdk";
import { SHORTCUT } from "@/plugins/sdk/kernelPoints";
import { chatSearchShortcut } from "./application/chatSearchShortcut";
import { openChatSearch } from "./application/openChatSearch";
import { ChatSearchOverlay } from "./ui/ChatSearchOverlay";

export default definePlugin({
  name: "lyra.builtin.chat-search",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("app.overlay", {
      id: "chat-search",
      order: 50,
      component: ChatSearchOverlay,
    });
    host.extensions.contribute(SHORTCUT, chatSearchShortcut(t, openChatSearch));
  },
});
