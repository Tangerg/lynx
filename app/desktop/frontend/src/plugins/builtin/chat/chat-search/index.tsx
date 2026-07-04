import { t } from "@/lib/i18n";
import { definePlugin } from "@/plugins/sdk";
import { SHORTCUT } from "@/plugins/sdk/kernelPoints";
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
    host.extensions.contribute(SHORTCUT, {
      key: "Mod+F",
      description: t("chatSearch.shortcutDesc"),
      // Users usually trigger chat search while focus is still in the composer.
      allowInInputs: true,
      handler: (e) => {
        e.preventDefault();
        openChatSearch();
      },
    });
  },
});
