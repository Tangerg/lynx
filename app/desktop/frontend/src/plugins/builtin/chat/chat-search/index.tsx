import { definePlugin } from "@/plugins/sdk";
import { chatSearchCommand, chatSearchOverlaySlot } from "./application/chatSearchContributions";
import { openChatSearch } from "./application/openChatSearch";
import { ChatSearchOverlay } from "./ui/ChatSearchOverlay";

export default definePlugin({
  name: "lyra.builtin.chat-search",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("app.overlay", chatSearchOverlaySlot(ChatSearchOverlay));
    host.commands.register(chatSearchCommand(openChatSearch));
  },
});
