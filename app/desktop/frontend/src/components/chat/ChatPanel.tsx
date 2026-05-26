// ChatPanel — the main pane.
//
// Thin orchestrator: render the top tab strip, then pick between the
// workspace-view body or the chat-stream body based on whether the
// user has promoted a workspace view tab. Both branches own their own
// state subscriptions; this file just owns the layout decision.
//
// Props are limited to one truly external input:
//   onSend — supplied by kernel-chat (knows how to forward into the
//            live AG-UI agent). Kept as a prop so ChatPanel itself
//            has no opinion about *how* messages get to the agent.

import type { ComposerMode } from "@/state/composerStore";
import { Panel } from "@/components/common";
import { useSessions } from "@/lib/queries";
import { useSessionStore } from "@/state/sessionStore";
import { ChatStream } from "./ChatStream";
import { PanelHeader } from "./PanelHeader";
import { WorkspaceViewBody } from "./WorkspaceViewBody";

interface Props {
  /** Send a plain user message through the live AG-UI agent. Supplied by
   *  kernel-chat (or whatever container owns the agent session). */
  onSend: (text: string) => void;
}

export function ChatPanel({ onSend }: Props) {
  const activeMainView = useSessionStore((s) => s.activeMainView);
  const activeSessionId = useSessionStore((s) => s.activeSessionId);
  const { data: sessions = [] } = useSessions();

  // Render nothing while sessions are still loading and there's no
  // workspace view to fall back to — avoids a blank-but-bordered panel.
  if (sessions.length === 0 && !activeMainView) return null;

  return (
    <Panel className="chat">
      <PanelHeader />
      {activeMainView ? (
        <WorkspaceViewBody viewId={activeMainView} />
      ) : (
        <ChatStream onSend={onSend} resetKey={activeSessionId} />
      )}
    </Panel>
  );
}

export type { ComposerMode };
