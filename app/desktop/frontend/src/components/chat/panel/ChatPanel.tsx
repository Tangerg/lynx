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

import type { ComposerMode } from "@/state/composerStore";
import type { ViewPlacement } from "./ViewPlacement";
import { Panel } from "@/components/common";
import { useSessions } from "@/lib/data/queries";
import { useWorkspaceViews } from "@/plugins/sdk";
import { useSessionStore } from "@/state/sessionStore";
import { useUiStore } from "@/state/uiStore";
import { ChatStream } from "./ChatStream";
import { PanelHeader } from "./PanelHeader";
import { SplitResizer } from "./SplitResizer";
import { ViewPlacementProvider } from "./ViewPlacement";
import { WorkspaceViewBody } from "./WorkspaceViewBody";

interface Props {
  /** Send a plain user message through the live agent. Supplied by
   *  kernel-chat (or whatever container owns the agent session). */
  onSend: (text: string) => void;
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
        // Chat | resizer | view. Each pane is its own `relative flex-col` so
        // ChatStream's absolute-positioned composer scopes to the chat half
        // (not the whole Panel) and both halves scroll independently.
        <div className="flex min-h-0 flex-1">
          <div
            className="relative flex min-h-0 min-w-0 flex-col"
            style={{ flexBasis: `${splitRatio * 100}%` }}
          >
            <ChatStream onSend={onSend} resetKey={activeSessionId} />
          </div>
          <SplitResizer />
          <div className="relative flex min-h-0 min-w-0 flex-1 flex-col">
            <ViewPlacementProvider value={placementFor(splitViewId, "split")}>
              <WorkspaceViewBody viewId={splitViewId} />
            </ViewPlacementProvider>
          </div>
        </div>
      ) : (
        <ChatStream onSend={onSend} resetKey={activeSessionId} />
      )}
    </Panel>
  );
}

export type { ComposerMode };
