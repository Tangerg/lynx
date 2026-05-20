// Built-in plugin: fills the "app.main" layout slot with the ChatPanel.
// Owns the AG-UI session lifecycle (useAgentSession) so the shell-level
// page component never has to know about agents at all.

import { useCallback, useEffect, useMemo } from "react";
import { ChatPanel } from "@/components/chat/ChatPanel";
import type { ChatTab } from "@/components/chat/ChatTopBar";
import { useSessions } from "@/lib/queries";
import { definePlugin, pickAgentSource } from "@/plugins/sdk";
import { useAgentStore } from "@/state/agentStore";
import { useComposerStore } from "@/state/composerStore";
import { useUIStore } from "@/state/uiStore";
import { useAgentSession } from "@/state/useAgentSession";

function ShellChat() {
  // ---------- UI store ----------
  const sidebarRail = useUIStore((s) => s.sidebarRail);
  const inspectorOpen = useUIStore((s) => s.inspectorOpen);
  const activeSession = useUIStore((s) => s.activeSessionId);
  const tabIds = useUIStore((s) => s.tabIds);
  const selectedToolId = useUIStore((s) => s.selectedToolId);
  const expandedToolIds = useUIStore((s) => s.expandedToolIds);
  const mainViewTabs = useUIStore((s) => s.mainViewTabs);
  const activeMainView = useUIStore((s) => s.activeMainView);
  const ui = useUIStore.getState();

  // ---------- Composer store ----------
  const composerValue = useComposerStore((s) => s.value);
  const composerMode = useComposerStore((s) => s.mode);
  const attachments = useComposerStore((s) => s.attachments);
  const composer = useComposerStore.getState();

  // ---------- Agent state ----------
  const messages = useAgentStore((s) => s.messages);
  const plan = useAgentStore((s) => s.plan);
  const toolCalls = useAgentStore((s) => s.toolCalls);

  // ---------- Sessions list ----------
  const { data: sessions = [] } = useSessions();
  const activeS = sessions.find((s) => s.id === activeSession) ?? sessions[0];
  const openTabs: ChatTab[] = useMemo(
    () =>
      tabIds
        .map((id) => sessions.find((s) => s.id === id))
        .filter((s): s is (typeof sessions)[number] => Boolean(s))
        .map((s) => ({ id: s.id, title: s.title, status: s.status })),
    [tabIds, sessions],
  );

  // ---------- AG-UI session ----------
  // The factory comes from the plugin-registered agent source — built-in
  // `lyra.builtin.http-agent` provides the default. Read non-reactively
  // (lookup once at mount); changing sources at runtime requires a reload,
  // same as other "boot-time" plugin configuration.
  const session = useAgentSession(
    useCallback(() => {
      const source = pickAgentSource();
      if (!source) throw new Error("No agent source registered");
      return source.factory();
    }, []),
  );

  // Auto-expand the latest tool the first time the conversation streams in.
  useEffect(() => {
    const tools = Object.values(toolCalls);
    if (tools.length > 0 && !ui.selectedToolId) {
      ui.expandTool(tools[tools.length - 1].id);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [toolCalls]);

  // Reads kept here so the wrapping `.app` CSS classes update reactively.
  void sidebarRail; void inspectorOpen;

  if (!activeS) return null;

  // Tool clicks pick the right inspector view, set context, then open it
  // as a main-area tab. There is no right pane anymore — VS Code style.
  const onOpenInspector = (toolId: string) => {
    const tool = toolCalls[toolId];
    if (!tool) return;

    let viewId = "diff";
    let title = "Diff";
    let icon = "diff";
    if (tool.fn === "bash") {
      viewId = "terminal"; title = "Terminal"; icon = "terminal";
    } else if (tool.fn === "edit_file" || tool.fn === "write_file" || tool.fn === "read_file") {
      viewId = "diff"; title = "Diff"; icon = "diff";
      const m = String(tool.args).match(/^([^\s(]+)/);
      if (m) ui.setActiveFile(m[1]);
    }
    ui.setInspectorTab(viewId as never);
    ui.setSelectedToolId(toolId);
    ui.openMainView({ id: viewId, title, icon });
  };

  // Clearing the textarea after a submit is owned by `submitComposer` so
  // both the Enter path and the send-button plugin behave identically;
  // this callback only has to forward the message to the agent.
  const onSend = (text: string) => session.send(text);

  return (
    <ChatPanel
      messages={messages}
      plan={plan}
      toolCalls={toolCalls}
      selectedToolId={selectedToolId}
      onSelectTool={ui.setSelectedToolId}
      composerValue={composerValue}
      onComposerChange={composer.setValue}
      onSend={onSend}
      attachments={attachments}
      onRemoveAttachment={composer.removeAttachment}
      mode={composerMode}
      onModeChange={composer.setMode}
      tabs={openTabs}
      viewTabs={mainViewTabs}
      activeTabId={activeSession}
      activeMainView={activeMainView}
      onSelectTab={(id) => { ui.selectChat(); ui.selectTab(id); }}
      onCloseTab={ui.closeTab}
      onSelectMainView={ui.selectMainView}
      onCloseMainView={ui.closeMainView}
      expandedToolIds={expandedToolIds}
      onToggleExpand={ui.toggleExpandedTool}
      onOpenInspector={onOpenInspector}
    />
  );
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
