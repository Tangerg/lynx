import { COMMAND, contributeLayout, definePlugin } from "@/plugins/sdk";
import { openChatSearch } from "./application/openChatSearch";
import { ChatSearchOverlay } from "./ui/ChatSearchOverlay";

export default definePlugin({
  name: "lyra.builtin.chat-search",
  setup(ctx) {
    contributeLayout(ctx, "app.overlay", {
      id: "chat-search",
      order: 50,
      component: ChatSearchOverlay,
    });
    // A command, not a bare shortcut, so it remains discoverable in the palette.
    ctx.contribute(COMMAND, {
      id: "chat.search",
      label: "command.chatSearch",
      icon: "search",
      group: "command.group.chat",
      keywords: ["find", "search", "conversation", "messages", "transcript"],
      order: 2,
      combo: "Mod+F",
      run: openChatSearch,
    });
  },
});
