import { COMMAND, definePlugin } from "@/plugins/sdk";
import {
  exportConversationJson,
  exportConversationMarkdown,
  importConversationJson,
} from "@/plugins/builtin/workspace/public/conversationArchive";
import { conversationExportCommands } from "./application/conversationExportCommands";

export default definePlugin({
  name: "lyra.builtin.conversation-export",
  setup(ctx) {
    for (const command of conversationExportCommands({
      exportMarkdown: exportConversationMarkdown,
      exportJson: exportConversationJson,
      importJson: importConversationJson,
    })) {
      ctx.contribute(COMMAND, command);
    }
  },
});
