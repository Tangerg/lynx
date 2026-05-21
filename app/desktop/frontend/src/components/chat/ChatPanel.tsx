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
   *  kernel-chat (or whatever container owns the agent session). */
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
  // Sticky-bottom auto-scroll.
  //
  // Semantics:
  //   1. DEFAULT: follow mode ON — new content auto-scrolls into view.
  //   2. User-initiated scroll away from the literal bottom → follow OFF.
  //      They stay parked wherever they stopped, no matter how much new
  //      content arrives.
  //   3. User-initiated scroll back to the literal bottom → follow ON.
  //
  // Key trick: distinguish *user* scrolls from *our own* programmatic
  // scrolls by sniffing the input device events (wheel / touchmove /
  // mousedown) that precede a real user scroll. We arm a short
  // userInputTimeout window when one of those fires; scroll events that
  // happen during that window are treated as user-initiated and update
  // follow mode. Scroll events outside the window (i.e. fired by our own
  // `scrollTop = scrollHeight` assignment from the ResizeObserver below)
  // are ignored, so we never accidentally toggle follow mode on
  // ourselves.
  //
  // ResizeObserver on the inner content fires for every height change —
  // backend deltas, smooth-text reveals, tool cards expanding — and
  // performs the actual auto-scroll only when follow mode is on.
  //
  // Threshold: only "literal max" counts as bottom (dist ≤ 1 for
  // sub-pixel rounding). The .msg-stream has 220px of bottom padding so
  // the floating composer sits over empty space; at max scroll the last
  // bubble lands right above the composer — that IS the visual bottom.
  const followRef = useRef(true);

  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;

    let userInputTimeout: number | null = null;
    const USER_INPUT_WINDOW_MS = 200;

    const markUserInput = () => {
      if (userInputTimeout !== null) clearTimeout(userInputTimeout);
      userInputTimeout = window.setTimeout(() => {
        userInputTimeout = null;
      }, USER_INPUT_WINDOW_MS);
    };

    // Wheel + touch cover trackpad, mouse-wheel, and touch-drag scrolls.
    // Mousedown catches scrollbar-thumb drags (and is a harmless no-op for
    // plain clicks since they don't move scrollTop).
    el.addEventListener("wheel", markUserInput, { passive: true });
    el.addEventListener("touchmove", markUserInput, { passive: true });
    el.addEventListener("mousedown", markUserInput);

    const onScroll = () => {
      // No recent user input → this scroll was driven by our own
      // `scrollTop = scrollHeight` below. Leave followRef alone.
      if (userInputTimeout === null) return;
      const dist = el.scrollHeight - el.scrollTop - el.clientHeight;
      followRef.current = dist <= 1;
    };
    el.addEventListener("scroll", onScroll, { passive: true });

    const ro = new ResizeObserver(() => {
      if (followRef.current) {
        el.scrollTop = el.scrollHeight;
      }
    });
    const inner = el.firstElementChild;
    if (inner) ro.observe(inner);

    return () => {
      el.removeEventListener("wheel", markUserInput);
      el.removeEventListener("touchmove", markUserInput);
      el.removeEventListener("mousedown", markUserInput);
      el.removeEventListener("scroll", onScroll);
      ro.disconnect();
      if (userInputTimeout !== null) clearTimeout(userInputTimeout);
    };
  }, []);

  // Session switch — always land at the bottom and re-arm follow mode.
  // The programmatic scroll below doesn't disengage follow because the
  // input-timeout gate in onScroll filters it out.
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    el.scrollTop = el.scrollHeight;
    followRef.current = true;
  }, [activeSession]);

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
          <div className="composer-wrap">
            <div className="composer-fade" />
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
