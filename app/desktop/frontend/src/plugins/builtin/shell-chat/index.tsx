// Built-in plugin: fills the "app.main" layout slot with ChatPanel.
//
// Responsibility is intentionally tiny — own the AG-UI session
// lifecycle and feed the `onSend` callback into ChatPanel. Everything
// else (state reads, view dispatch, tool-click routing) is owned by
// ChatPanel itself reading from stores, or by the
// `openInspectorFromTool` utility that ToolCard invokes directly.

import { ChatPanel } from "@/components/chat/ChatPanel";
import { definePlugin } from "@/plugins/sdk";
import { useDefaultChatSession } from "@/state/useDefaultChatSession";

function ShellChat() {
  const session = useDefaultChatSession();
  // Clearing the textarea after submit is owned by `submitComposer` so
  // this callback only has to forward into the live agent.
  return <ChatPanel onSend={session.send} />;
}

export default definePlugin({
  name: "lyra.builtin.shell-chat",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("app.main", {
      id: "chat",
      order: 0,
      component: ShellChat,
    });
  },
});
