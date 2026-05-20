// ChatPanel — the main pane.
//
// Reads everything it can from the application stores (useAgentStore,
// useUIStore, useComposerStore, useSessions) so the shell wrapper
// (shell-chat plugin) shrinks to just the agent-session lifecycle.
//
// Props are limited to two truly external inputs:
//   `onSend`     — supplied by shell-chat (knows how to forward into the
//                  live AG-UI agent). Kept as a prop so ChatPanel itself
//                  has no opinion about *how* messages get to the agent.
//   `model`      — was already optional; the composer-toolbar plugin
//                  reads it from useSessions directly, so we don't even
//                  pass it here.
//
// Everything else (messages, tabs, view-tabs, composer state, ui flags)
// is read where it's needed via store hooks.

import { useEffect, useMemo, useRef } from "react";
import { Panel } from "@/components/common";
import { InspectorProvider } from "@/components/inspector/InspectorContext";
import { PluginBoundary } from "@/plugins/PluginBoundary";
import { Slot } from "@/plugins/Slot";
import { useInspectorTabs, usePluginStore } from "@/plugins/sdk";
import { useSessions } from "@/lib/queries";
import { useAgentStore } from "@/state/agentStore";
import { useComposerStore } from "@/state/composerStore";
import { useUIStore } from "@/state/uiStore";
import { ChatTopBar, type ChatTab } from "./ChatTopBar";
import { MessageStream } from "./MessageStream";
import { SlashSuggestions } from "./SlashSuggestions";
import { Composer, type ComposerMode } from "./Composer";
import { ComposerFooter } from "./ComposerFooter";

type Props = {
  /** Send a plain user message through the live AG-UI agent. Supplied by
   *  shell-chat (or whatever container owns the agent session). */
  onSend: (text: string) => void;
};

export function ChatPanel({ onSend }: Props) {
  const scrollRef = useRef<HTMLDivElement>(null);

  // ---- agent state ----
  const messages = useAgentStore((s) => s.messages);
  const plan = useAgentStore((s) => s.plan);
  const toolCalls = useAgentStore((s) => s.toolCalls);

  // ---- ui state ----
  const activeSession = useUIStore((s) => s.activeSessionId);
  const tabIds = useUIStore((s) => s.tabIds);
  const selectedToolId = useUIStore((s) => s.selectedToolId);
  const expandedToolIds = useUIStore((s) => s.expandedToolIds);
  const mainViewTabs = useUIStore((s) => s.mainViewTabs);
  const activeMainView = useUIStore((s) => s.activeMainView);

  // ---- composer state ----
  const composerValue = useComposerStore((s) => s.value);
  const composerMode = useComposerStore((s) => s.mode);
  const attachments = useComposerStore((s) => s.attachments);

  // ---- sessions list ----
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

  // ---- side effects ----
  // Auto-scroll to bottom on new messages.
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
  }, [messages]);

  // Auto-expand the latest tool the first time it streams in. Pure
  // ui-state side effect; lives here because it's chat-specific.
  useEffect(() => {
    const tools = Object.values(toolCalls);
    const ui = useUIStore.getState();
    if (tools.length > 0 && !ui.selectedToolId) {
      ui.expandTool(tools[tools.length - 1].id);
    }
  }, [toolCalls]);

  // Resolve the active main-view body. Workspace registry takes priority;
  // inspector tabs are auto-promoted as a fallback.
  const inspectorTabs = useInspectorTabs();
  const activeViewBody = activeMainView
    ? (usePluginStore.getState().workspaceViews.get(activeMainView)?.value.component
       ?? inspectorTabs.find((t) => t.id === activeMainView)?.component
       ?? null)
    : null;
  const headerActiveId = activeMainView ?? activeSession;

  if (!activeS) return null;

  return (
    <Panel className="chat">
      <ChatTopBar
        tabs={openTabs}
        viewTabs={mainViewTabs}
        activeId={headerActiveId}
        onSelectChat={selectChat}
        onCloseChat={useUIStore.getState().closeTab}
        onSelectView={useUIStore.getState().selectMainView}
        onCloseView={useUIStore.getState().closeMainView}
      />

      {activeViewBody ? (
        // Workspace view tab (Settings, Diff, Files, etc.) — full-bleed,
        // no composer underneath. Whatever the view needs is its own
        // problem; the chat composer is irrelevant here.
        <PluginBoundary plugin={`workspace:${activeMainView}`} label="main view">
          <MainViewInspectorBridge>
            <ActiveView component={activeViewBody} />
          </MainViewInspectorBridge>
        </PluginBoundary>
      ) : (
        <>
          <MessageStream
            ref={scrollRef}
            messages={messages}
            ctx={{
              plan,
              toolCalls,
              selectedToolId,
              onSelectTool: useUIStore.getState().setSelectedToolId,
              expandedIds: expandedToolIds,
              onToggleExpand: useUIStore.getState().toggleExpandedTool,
            }}
          />
          <div className="composer-wrap">
            <div className="composer-fade" />
            <div className="composer-inner">
              <Slot name="chat.status" />
              <SlashSuggestions value={composerValue} onPick={useComposerStore.getState().setValue} />
              <Composer
                value={composerValue}
                onChange={useComposerStore.getState().setValue}
                onSend={onSend}
                attachments={attachments}
                onRemoveAttachment={useComposerStore.getState().removeAttachment}
                mode={composerMode}
                onModeChange={useComposerStore.getState().setMode}
              />
              <ComposerFooter />
            </div>
          </div>
        </>
      )}
    </Panel>
  );
}

// Switching to a chat session has to clear `activeMainView` first so the
// workspace-view tab doesn't stay focused.
function selectChat(id: string) {
  const ui = useUIStore.getState();
  ui.selectChat();
  ui.selectTab(id);
}

// Trivial wrapper — React likes a stable component reference for the
// dynamic body we resolved imperatively.
function ActiveView({ component: Body }: { component: React.ComponentType }) {
  return <Body />;
}

// Supplies the InspectorContext when an inspector tab is promoted into
// the main pane. Reads `activeFile` / setters from the same store the
// docked inspector used to, so the inner tab body keeps working.
function MainViewInspectorBridge({ children }: { children: React.ReactNode }) {
  const activeFile = useUIStore((s) => s.activeFile);
  const setActiveFile = useUIStore((s) => s.setActiveFile);
  const setInspectorTab = useUIStore((s) => s.setInspectorTab);
  return (
    <InspectorProvider
      value={{ activeFile, onSelectFile: setActiveFile, onSwitchTab: setInspectorTab }}
    >
      {children}
    </InspectorProvider>
  );
}

export type { ComposerMode };
