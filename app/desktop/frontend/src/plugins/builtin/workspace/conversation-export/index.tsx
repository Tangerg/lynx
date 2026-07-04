import { t } from "@/lib/i18n";
import { definePlugin } from "@/plugins/sdk";
import {
  exportConversationJson,
  exportConversationMarkdown,
  importConversationJson,
} from "@/plugins/builtin/workspace/public/conversationArchive";
import { conversationExportCommands } from "./application/conversationExportCommands";

export default definePlugin({
  name: "lyra.builtin.conversation-export",
  version: "1.0.0",
  requires: ["lyra.builtin.workspace-bootstrap"],
  setup({ host }) {
    for (const command of conversationExportCommands(t, {
      exportMarkdown: exportConversationMarkdown,
      exportJson: exportConversationJson,
      importJson: importConversationJson,
    })) {
      host.commands.register(command);
    }
  },
});
