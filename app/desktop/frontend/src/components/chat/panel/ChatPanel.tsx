// ChatPanel — the main pane.
//
// Thin orchestrator: render the top tab strip, then pick between the
// workspace-view body or the chat-stream body based on whether the
// user has promoted a workspace view tab. Both branches own their own
// state subscriptions; this file just owns the layout decision.
//
// Props are limited to one truly external input:
//   onSend — supplied by kernel-chat (knows how to forward into the
//            live agent). Kept as a prop so ChatPanel itself
//            has no opinion about *how* messages get to the agent.

import type { UserInput } from "@/lib/agent/composerInput";
import type { ViewPlacement } from "./ViewPlacement";
import { Panel } from "@/components/common";
import { useSessions } from "@/lib/data/queries";
import { useWorkspaceViews } from "@/plugins/sdk";
import { useSessionStore } from "@/state/sessionStore";
import { useUiStore } from "@/state/uiStore";
import { ChatStream } from "./ChatStream";
import { MainSplit } from "./MainSplit";
import { PanelHeader } from "./PanelHeader";
import { ViewPlacementProvider } from "./ViewPlacement";
import { WorkspaceViewBody } from "./WorkspaceViewBody";

interface Props {
  /** Send the user's message input (text + inlined images) through the live
   *  agent. Supplied by kernel-chat (or whatever container owns the session). */
  onSend: (input: UserInput) => void;
}

export function ChatPanel({ onSend }: Props) {
  const activeMainView = useSessionStore((s) => s.activeMainView);
  const splitViewId = useSessionStore((s) => s.splitViewId);
  const activeSessionId = useSessionStore((s) => s.activeSessionId);
  const splitRatio = useUiStore((s) => s.splitRatio);
  const views = useWorkspaceViews();
  const { isLoading } = useSessions();

  // Suppress the panel only while the FIRST sessions fetch is in flight (and
  // no workspace view is promoted) — avoids a blank-but-bordered flash. Once
  // loaded, render even with ZERO sessions: ChatStream shows the welcome
  // screen + composer, which is the empty-state entry point (sending there
  // spins up a session via useChatSend). Returning null on empty stranded
  // the user with a blank main area and no way to start.
  if (isLoading && !activeMainView && !splitViewId) return null;

  // Placement controls handed to a promoted view's own ViewHeader (via the
  // ViewPlacement context) so it can move itself full ↔ beside-chat / close,
  // without ChatPanel reaching into the view body or the tab strip.
  const placementFor = (id: string, placement: "full" | "split"): ViewPlacement => {
    const spec = views.find((v) => v.id === id);
    const tab = { id, title: spec?.title ?? id, icon: spec?.icon };
    const store = () => useSessionStore.getState();
    return {
      placement,
      splittable: spec?.splittable ?? false,
      onSplit: () => store().openMainViewBeside(tab),
      onPromote: () => store().promoteSplitToTab(),
      onClose: () => (placement === "split" ? store().closeSplit() : store().closeMainView(id)),
    };
  };

  return (
    // No `container-type: inline-size` here — it implicitly enables layout
    // containment, which interacted badly with use-stick-to-bottom (the lib's
    // ResizeObserver + scroll-anchor path lost position during streaming and
    // snapped the chat to the top). MessageOutline's visibility keys off a
    // viewport media query instead of a container query — see MessageOutline.
    <Panel className="relative">
      <PanelHeader />
      {activeMainView ? (
        <ViewPlacementProvider value={placementFor(activeMainView, "full")}>
          <WorkspaceViewBody viewId={activeMainView} />
        </ViewPlacementProvider>
      ) : splitViewId ? (
        <MainSplit
          onSend={onSend}
          sessionId={activeSessionId}
          viewId={splitViewId}
          placement={placementFor(splitViewId, "split")}
          ratio={splitRatio}
        />
      ) : (
        <ChatStream onSend={onSend} resetKey={activeSessionId} />
      )}
    </Panel>
  );
}
