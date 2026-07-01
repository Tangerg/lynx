import { t } from "@/lib/i18n";
import { definePlugin } from "@/plugins/sdk";
import {
  exportConversationJson,
  exportConversationMarkdown,
  importConversationJson,
} from "@/plugins/builtin/workspace/application/conversationExport";
import { installConversationArchiveGateway } from "@/plugins/builtin/workspace/adapters/runtimeConversationArchiveGateway";

export default definePlugin({
  name: "lyra.builtin.conversation-export",
  version: "1.0.0",
  setup({ host }) {
    installConversationArchiveGateway();
    host.commands.register({
      id: "chat.export.markdown",
      label: t("convExport.markdown"),
      icon: "filetext",
      group: "Chat",
      keywords: ["save", "download", "export"],
      run: exportConversationMarkdown,
    });
    host.commands.register({
      id: "chat.export.json",
      label: t("convExport.json"),
      icon: "code",
      group: "Chat",
      keywords: ["save", "download", "export", "archive"],
      run: exportConversationJson,
    });
    host.commands.register({
      id: "chat.import.json",
      label: t("convExport.import"),
      icon: "history",
      group: "Chat",
      keywords: ["restore", "load", "import"],
      run: importConversationJson,
    });
  },
});
