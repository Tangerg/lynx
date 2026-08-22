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
    // A command keeps the shortcut description and handler in one declaration.
    ctx.contribute(COMMAND, {
      id: "chat.search",
      label: "command.chatSearch",
      combo: "Mod+F",
      run: openChatSearch,
    });
  },
});
