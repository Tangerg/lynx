// ChatPanel — the main pane.
//
// Reads everything it can from the application stores (useAgentStore,
// useUIStore, useComposerStore, useSessions) so the kernel wrapper
// (kernel-chat plugin) shrinks to just the agent-session lifecycle.
//
// Props are limited to one truly external input:
//   onSend — supplied by kernel-chat (knows how to forward into the
//            live AG-UI agent). Kept as a prop so ChatPanel itself has
//            no opinion about *how* messages get to the agent.

import { useEffect, useMemo, useRef } from "react";
import { Panel } from "@/components/common";
import { PluginBoundary } from "@/plugins/PluginBoundary";
import { Slot } from "@/plugins/Slot";
import { useWorkspaceViews } from "@/plugins/sdk";
import { useSessions } from "@/lib/queries";
import { useStickyBottomScroll } from "@/lib/useStickyBottomScroll";
import { useAgentSlice } from "@/state/agentStore";
import { useComposerStore } from "@/state/composerStore";
import { useUIStore } from "@/state/uiStore";
import { ChatErrorBoundary } from "./ChatErrorBoundary";
import { ChatTopBar, type ChatTab } from "./ChatTopBar";
import { JumpToBottomButton } from "./JumpToBottomButton";
import { MessageStream } from "./MessageStream";
import { SlashSuggestions } from "./SlashSuggestions";
import { Composer, type ComposerMode } from "./Composer";
import { ComposerFooter } from "./ComposerFooter";

type Props = {
  /** Send a plain user message through the live AG-UI agent. Supplied by
   *  kernel-chat (or whatever container owns the agent session). */
  onSend: (text: string) => void;
};

export function ChatPanel({ onSend }: Props) {
  const scrollRef = useRef<HTMLDivElement>(null);

  // ---- agent state (scoped to the current session) ----
  const messages = useAgentSlice((v) => v.messages);
  const plan = useAgentSlice((v) => v.plan);
  const toolCalls = useAgentSlice((v) => v.toolCalls);

  // ---- ui state ----
  const activeSession = useUIStore((s) => s.activeSessionId);
  const tabIds = useUIStore((s) => s.tabIds);
  const selectedToolId = useUIStore((s) => s.selectedToolId);
  const expandedToolIds = useUIStore((s) => s.expandedToolIds);
  const mainViewTabs = useUIStore((s) => s.mainViewTabs);
  const activeMainView = useUIStore((s) => s.activeMainView);

  // ---- ui actions ----
  const closeTab = useUIStore((s) => s.closeTab);
  const selectMainView = useUIStore((s) => s.selectMainView);
  const closeMainView = useUIStore((s) => s.closeMainView);
  const setSelectedToolId = useUIStore((s) => s.setSelectedToolId);
  const toggleExpandedTool = useUIStore((s) => s.toggleExpandedTool);

  // ---- composer state ----
  const composerValue = useComposerStore((s) => s.value);
  const composerMode = useComposerStore((s) => s.mode);
  const attachments = useComposerStore((s) => s.attachments);

  // ---- composer actions ----
  const setComposerValue = useComposerStore((s) => s.setValue);
  const setComposerMode = useComposerStore((s) => s.setMode);
  const removeAttachment = useComposerStore((s) => s.removeAttachment);

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
  // Sticky-bottom auto-scroll. Hook owns: scroll/wheel/touch/mousedown
  // listeners, the user-vs-programmatic-scroll discrimination, and the
  // ResizeObserver that triggers follow scrolls on content growth.
  // `activeSession` resets follow mode + snaps to bottom on session swap.
  // Returns `atBottom` (mirrored React state) so we can show / hide the
  // jump-to-bottom button, plus `scrollToBottom` for that button's click.
  const { atBottom, scrollToBottom } = useStickyBottomScroll(scrollRef, activeSession);

  // Auto-select (but don't expand) the latest tool the first time it
  // streams in — so the inspector pane has something to show without
  // forcing the inline card to balloon open. Expanding is a deliberate
  // user click.
  //
  // We snapshot UI state via getState() instead of subscribing — we want
  // this effect to fire only when the toolCalls map mutates, not when
  // the user manually selects a tool.
  useEffect(() => {
    const tools = Object.values(toolCalls);
    const ui = useUIStore.getState();
    if (tools.length > 0 && !ui.selectedToolId) {
      ui.setSelectedToolId(tools[tools.length - 1].id);
    }
  }, [toolCalls]);

  // Resolve the active main-view body via the workspace registry.
  const workspaceViews = useWorkspaceViews();
  const activeViewBody = activeMainView
    ? workspaceViews.find((v) => v.id === activeMainView)?.component ?? null
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
        onCloseChat={closeTab}
        onSelectView={selectMainView}
        onCloseView={closeMainView}
      />

      {activeViewBody ? (
        // Workspace view tab (Settings, Diff, Files, …) — full-bleed,
        // no composer. Whatever the view needs is its own problem.
        <PluginBoundary plugin={`workspace:${activeMainView}`} label="main view">
          <ActiveView component={activeViewBody} />
        </PluginBoundary>
      ) : (
        <>
          <ChatErrorBoundary
            resetKey={activeSession}
            label={`session:${activeSession}`}
          >
            <MessageStream
              ref={scrollRef}
              messages={messages}
              ctx={{
                plan,
                toolCalls,
                selectedToolId,
                onSelectTool: setSelectedToolId,
                expandedIds: expandedToolIds,
                onToggleExpand: toggleExpandedTool,
              }}
            />
          </ChatErrorBoundary>
          <div className="composer-wrap">
            <div className="composer-fade" />
            <JumpToBottomButton visible={!atBottom} onClick={scrollToBottom} />
            <div className="composer-inner">
              <Slot name="chat.status" />
              <SlashSuggestions value={composerValue} onPick={setComposerValue} />
              <Composer
                value={composerValue}
                onChange={setComposerValue}
                onSend={onSend}
                attachments={attachments}
                onRemoveAttachment={removeAttachment}
                mode={composerMode}
                onModeChange={setComposerMode}
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

export type { ComposerMode };
