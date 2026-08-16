import { COMMAND, contributeLayout, definePlugin } from "@/plugins/sdk";
import { chatSearchCommand, chatSearchOverlaySlot } from "./application/chatSearchContributions";
import { openChatSearch } from "./application/openChatSearch";
import { ChatSearchOverlay } from "./ui/ChatSearchOverlay";

export default definePlugin({
  name: "lyra.builtin.chat-search",
  setup(ctx) {
    contributeLayout(ctx, "app.overlay", chatSearchOverlaySlot(ChatSearchOverlay));
    ctx.contribute(COMMAND, chatSearchCommand(openChatSearch));
  },
});
