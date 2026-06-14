// ChatStream — the message stream + composer surface.
//
// Owns the agent / session / composer state slices it actually reads
// (no fat shared interface), the auto-select-latest-tool effect, and
// the streamControls bridge that lets the jump-to-bottom button know
// when the user has scrolled away from the tail.

import type { StreamControls } from "./MessageStream";
import type { UserInput } from "@/lib/agent/composerInput";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useModels } from "@/lib/data/queries";
import { useActiveSession } from "@/lib/agent/useActiveSession";
import { Slot } from "@/plugins/host/Slot";
import { useAgentSlice } from "@/state/agentStore";
import { useComposerStore } from "@/state/composerStore";
import { useSessionStore } from "@/state/sessionStore";
import { useUiStore } from "@/state/uiStore";
import { Composer, ComposerFooter, SlashSuggestions } from "../composer";
import { ChatErrorBoundary } from "./ChatErrorBoundary";
import { CwdMissingBanner } from "./CwdMissingBanner";
import { JumpToBottomButton } from "./JumpToBottomButton";
import { MessageStream } from "./MessageStream";
import { RunErrorBanner } from "./RunErrorBanner";

interface Props {
  /** Send the user's message input (text + inlined images) through the live agent. */
  onSend: (input: UserInput) => void;
  /** Active session id — used to reset the error boundary + stream. */
  resetKey: string;
}

export function ChatStream({ onSend, resetKey }: Props) {
  const messages = useAgentSlice((v) => v.messages);
  const plan = useAgentSlice((v) => v.plan);
  const toolCalls = useAgentSlice((v) => v.toolCalls);

  const selectedToolId = useSessionStore((s) => s.selectedToolId);
  const expandedToolIds = useSessionStore((s) => s.expandedToolIds);
  const setSelectedToolId = useSessionStore((s) => s.setSelectedToolId);
  const toggleExpandedTool = useSessionStore((s) => s.toggleExpandedTool);

  const composerValue = useComposerStore((s) => s.value);
  const composerMode = useComposerStore((s) => s.mode);
  const images = useComposerStore((s) => s.images);
  const setComposerValue = useComposerStore((s) => s.setValue);
  const setComposerMode = useComposerStore((s) => s.setMode);
  const removeImage = useComposerStore((s) => s.removeImage);
  const clearComposer = useComposerStore((s) => s.clear);
  const addImageFiles = useComposerStore((s) => s.addImageFiles);

  // Assistant header name = this session's model, resolved to its friendly
  // displayName. Resolved ONCE here (both lists are stable react-query data,
  // not per-token state) and threaded down — never per-message subscriptions
  // (CLAUDE.md §5). Falls back to the raw model id, then the role's neutral
  // "Assistant" in MessageBlock.
  const { data: models } = useModels();
  const sessionModelId = useActiveSession()?.model ?? "";
  const assistantName = useMemo(() => {
    if (!sessionModelId) return undefined;
    return models?.find((m) => m.id === sessionModelId)?.label ?? sessionModelId;
  }, [sessionModelId, models]);

  // Global streaming-reveal preference. Read once here (stable string) and
  // threaded through ctx so MarkdownMessage stays prop-driven — no per-block
  // store subscription on the hot streaming path.
  const typewriter = useUiStore((s) => s.streamReveal) === "typewriter";

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
  // Effect deps narrow to `latestToolId` (a string, stable under
  // Object.is) so it only fires when the *latest* tool changes —
  // not on every TOOL_CALL_ARGS delta that mutates the toolCalls map
  // reference while leaving the latest id alone.
  const latestToolId = useMemo(() => Object.keys(toolCalls).at(-1) ?? "", [toolCalls]);
  useEffect(() => {
    if (!latestToolId) return;
    const ui = useSessionStore.getState();
    if (!ui.selectedToolId) ui.setSelectedToolId(latestToolId);
  }, [latestToolId]);

  // Stable ctx identity — without useMemo, this object literal is
  // recreated on every render, which (combined with the React.memo on
  // MessageBlock) would kick every message in the stream into a fresh
  // render path on every token delta. Memoised, the ref only changes
  // when one of the underlying slices actually changes, so pure text
  // streaming (no tool / plan churn) keeps non-tail messages off the
  // render path entirely.
  const ctx = useMemo(
    () => ({
      plan,
      toolCalls,
      selectedToolId,
      onSelectTool: setSelectedToolId,
      expandedIds: expandedToolIds,
      onToggleExpand: toggleExpandedTool,
      typewriter,
    }),
    [
      plan,
      toolCalls,
      selectedToolId,
      setSelectedToolId,
      expandedToolIds,
      toggleExpandedTool,
      typewriter,
    ],
  );

  return (
    <>
      {/* Keyed on the session so the relocate input never carries a
          half-typed path across a tab switch. */}
      <CwdMissingBanner key={resetKey} />
      <RunErrorBanner />
      {/* Banner row pinned above the message stream — sits at the
          chat header's lower edge and stays put while the user scrolls
          messages below (the scroll lives inside MessageStream's own
          container, not this one). Plan-progress is the only built-in
          contributor today; the slot is open so plugins can stack
          their own "above the stream" banners here. */}
      <div className="pointer-events-auto mx-auto w-full max-w-[760px] px-6">
        <Slot name="chat.banner.top" />
      </div>
      <ChatErrorBoundary resetKey={resetKey} label={`session:${resetKey}`}>
        <MessageStream
          messages={messages}
          ctx={ctx}
          assistantName={assistantName}
          resetKey={resetKey}
          onControlsChange={handleControls}
        />
      </ChatErrorBoundary>
      <div className="pointer-events-none absolute inset-x-0 bottom-0 px-6 pb-4">
        <div
          className="pointer-events-none absolute inset-x-0 bottom-0 h-[140px]"
          style={{
            background:
              "linear-gradient(180deg, color-mix(in oklab, var(--color-surface) 0%, transparent) 0%, var(--color-surface) 50%)",
          }}
        />
        <JumpToBottomButton
          visible={streamControls ? !streamControls.isAtBottom : false}
          onClick={() => streamControls?.scrollToBottom()}
        />
        {/* px-6 mirrors msg-stream's 24px content padding so the
            composer's outer edge lines up with the message text
            column above it (instead of the raw 760px column edge). */}
        <div className="pointer-events-auto relative z-[2] mx-auto w-full max-w-[760px] px-6">
          <Slot name="chat.status" />
          <SlashSuggestions value={composerValue} onPick={setComposerValue} />
          <Composer
            value={composerValue}
            onChange={setComposerValue}
            onClear={clearComposer}
            onSend={onSend}
            images={images}
            onRemoveImage={removeImage}
            onAddImages={addImageFiles}
            mode={composerMode}
            onModeChange={setComposerMode}
          />
          <ComposerFooter />
        </div>
      </div>
    </>
  );
}
