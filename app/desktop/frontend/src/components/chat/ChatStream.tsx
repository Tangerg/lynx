// ChatStream — the message stream + composer surface.
//
// Owns the agent / session / composer state slices it actually reads
// (no fat shared interface), the auto-select-latest-tool effect, and
// the streamControls bridge that lets the jump-to-bottom button know
// when the user has scrolled away from the tail.

import { useCallback, useEffect, useState } from "react";
import { Slot } from "@/plugins/Slot";
import { useAgentSlice } from "@/state/agentStore";
import { useComposerStore } from "@/state/composerStore";
import { useSessionStore } from "@/state/sessionStore";
import { ChatErrorBoundary } from "./ChatErrorBoundary";
import { Composer } from "./Composer";
import { ComposerFooter } from "./ComposerFooter";
import { JumpToBottomButton } from "./JumpToBottomButton";
import { MessageStream, type StreamControls } from "./MessageStream";
import { RunErrorBanner } from "./RunErrorBanner";
import { SlashSuggestions } from "./SlashSuggestions";

type Props = {
  /** Send a plain user message through the live AG-UI agent. */
  onSend: (text: string) => void;
  /** Active session id — used to reset the error boundary + stream. */
  resetKey: string;
};

export function ChatStream({ onSend, resetKey }: Props) {
  // ---- agent state (scoped to the current session) ----
  const messages = useAgentSlice((v) => v.messages);
  const plan = useAgentSlice((v) => v.plan);
  const toolCalls = useAgentSlice((v) => v.toolCalls);

  // ---- session UI: tool inspector ----
  const selectedToolId = useSessionStore((s) => s.selectedToolId);
  const expandedToolIds = useSessionStore((s) => s.expandedToolIds);
  const setSelectedToolId = useSessionStore((s) => s.setSelectedToolId);
  const toggleExpandedTool = useSessionStore((s) => s.toggleExpandedTool);

  // ---- composer ----
  const composerValue = useComposerStore((s) => s.value);
  const composerMode = useComposerStore((s) => s.mode);
  const attachments = useComposerStore((s) => s.attachments);
  const setComposerValue = useComposerStore((s) => s.setValue);
  const setComposerMode = useComposerStore((s) => s.setMode);
  const removeAttachment = useComposerStore((s) => s.removeAttachment);

  // Sticky-bottom auto-scroll lives inside MessageStream via
  // `use-stick-to-bottom`. This component only needs to know "is the
  // user currently at bottom?" to toggle the jump-to-bottom button.
  const [streamControls, setStreamControls] = useState<StreamControls | null>(null);
  const handleControls = useCallback((c: StreamControls) => setStreamControls(c), []);

  // Auto-select (but don't expand) the latest tool the first time it
  // streams in — so the inspector pane has something to show without
  // forcing the inline card to balloon open. Expanding is a deliberate
  // user click.
  //
  // Snapshot via getState() instead of subscribing so this effect fires
  // only when the toolCalls map mutates, not when the user picks a tool.
  useEffect(() => {
    const tools = Object.values(toolCalls);
    const ui = useSessionStore.getState();
    if (tools.length > 0 && !ui.selectedToolId) {
      ui.setSelectedToolId(tools[tools.length - 1].id);
    }
  }, [toolCalls]);

  return (
    <>
      <RunErrorBanner />
      <ChatErrorBoundary resetKey={resetKey} label={`session:${resetKey}`}>
        <MessageStream
          messages={messages}
          ctx={{
            plan,
            toolCalls,
            selectedToolId,
            onSelectTool: setSelectedToolId,
            expandedIds: expandedToolIds,
            onToggleExpand: toggleExpandedTool,
          }}
          resetKey={resetKey}
          onControlsChange={handleControls}
        />
      </ChatErrorBoundary>
      <div className="pointer-events-none absolute inset-x-0 bottom-0 px-6 pb-4">
        <div
          className="pointer-events-none absolute inset-x-0 bottom-0 h-[140px]"
          style={{
            background:
              "linear-gradient(180deg, color-mix(in srgb, var(--color-surface) 0%, transparent) 0%, var(--color-surface) 50%)",
          }}
        />
        <JumpToBottomButton
          visible={streamControls ? !streamControls.isAtBottom : false}
          onClick={() => streamControls?.scrollToBottom()}
        />
        <div className="pointer-events-auto relative z-[2] mx-auto w-full max-w-[760px]">
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
  );
}
