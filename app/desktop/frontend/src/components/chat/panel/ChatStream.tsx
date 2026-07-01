// ChatStream — the message stream + composer surface.
//
// Owns the agent / session / composer state slices it actually reads
// (no fat shared interface), the auto-select-latest-tool effect, and
// the streamControls bridge that lets the jump-to-bottom button know
// when the user has scrolled away from the tail.

import type { StreamControls } from "./MessageStream";
import type { UserInput } from "@/plugins/builtin/chat/composer/public/input";
import { useCallback, useEffect, useMemo, useState } from "react";
import { useActiveConversationMessages } from "@/plugins/builtin/agent/public/conversation";
import { useActiveRunPlan, useActiveRunToolCalls } from "@/plugins/builtin/agent/public/run";
import { useActiveSessionId } from "@/plugins/builtin/agent/public/session";
import { useSelectedModel } from "@/plugins/builtin/chat/composer/public/selectedModel";
import { useT } from "@/lib/i18n";
import { Slot } from "@/plugins/host/Slot";
import {
  useAddComposerImageFiles,
  useAddComposerPaste,
  useComposerImages,
  useComposerPastes,
  useRemoveComposerImage,
  useRemoveComposerPaste,
} from "@/plugins/builtin/chat/composer/public/attachments";
import {
  useClearComposerDraft,
  useComposerText,
  useSetComposerText,
} from "@/plugins/builtin/chat/composer/public/draft";
import {
  selectInitialWorkspaceTool,
  useExpandedWorkspaceToolIds,
  useSelectWorkspaceTool,
  useToggleWorkspaceTool,
} from "@/plugins/builtin/workspace/public/navigation";
import { useUiStore } from "@/state/uiStore";
import { Composer, ComposerFooter, SlashSuggestions } from "@/plugins/builtin/chat/composer/ui";
import { ChatErrorBoundary } from "./ChatErrorBoundary";
import { CwdMissingBanner } from "./CwdMissingBanner";
import { JumpToBottomButton } from "./JumpToBottomButton";
import { MessageStream } from "./MessageStream";
import { RunErrorBanner } from "./RunErrorBanner";

interface Props {
  /** Send the user's message input (text + inlined images) through the live agent. */
  onSend: (input: UserInput) => void;
}

export function ChatStream({ onSend }: Props) {
  const resetKey = useActiveSessionId();
  const messages = useActiveConversationMessages();
  const plan = useActiveRunPlan();
  const toolCalls = useActiveRunToolCalls();

  const expandedToolIds = useExpandedWorkspaceToolIds();
  const selectTool = useSelectWorkspaceTool();
  const toggleExpandedTool = useToggleWorkspaceTool();

  const composerValue = useComposerText();
  const images = useComposerImages();
  const setComposerValue = useSetComposerText();
  const removeImage = useRemoveComposerImage();
  const clearComposer = useClearComposerDraft();
  const addImageFiles = useAddComposerImageFiles();
  const pastes = useComposerPastes();
  const removePaste = useRemoveComposerPaste();
  const addPaste = useAddComposerPaste();
  // Gate image staging on the next run's model accepting images — keeps the
  // paste/drop path consistent with the (disabled) toolbar attach button.
  const acceptsImages = useSelectedModel()?.multimodal ?? false;

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
    selectInitialWorkspaceTool(latestToolId);
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
      onSelectTool: selectTool,
      expandedIds: expandedToolIds,
      onToggleExpand: toggleExpandedTool,
      typewriter,
    }),
    [plan, toolCalls, selectTool, expandedToolIds, toggleExpandedTool, typewriter],
  );

  // The composer surface (status + slash hints + input + footer) — shared by
  // the empty-state centered layout and the normal bottom-anchored one.
  // Context chips (ComposerFooter) are rendered INSIDE the composer container
  // so the input + pills + toolbar form one unified surface (craft-style).
  const composer = (
    <>
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
        pastes={pastes}
        onRemovePaste={removePaste}
        onAddPaste={addPaste}
        acceptsImages={acceptsImages}
      >
        <ComposerFooter />
      </Composer>
    </>
  );

  const t = useT();

  // Empty state (Codex / ChatGPT voice): the hero + composer are ONE
  // vertically-centered group. No MessageStream / StickToBottom here — nothing
  // is streaming yet, so the delicate sticky-scroll path only mounts once there
  // are messages, and the composer "drops" to the bottom on the first send.
  if (messages.length === 0) {
    return (
      <>
        <CwdMissingBanner key={resetKey} />
        <RunErrorBanner />
        <div className="panel-scroll flex flex-1 flex-col items-center justify-center overflow-y-auto px-4">
          <h1 className="text-center text-[30px] font-medium tracking-normal text-fg">
            {t("welcome.title")}
          </h1>
          <div className="mt-6 w-full max-w-[760px]">
            <Slot name="chat.empty" />
          </div>
          <div className="mt-5 w-full max-w-[800px]">{composer}</div>
        </div>
      </>
    );
  }

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
      <div className="pointer-events-auto mx-auto w-full max-w-[840px] px-5">
        <Slot name="chat.banner.top" />
      </div>
      <ChatErrorBoundary resetKey={resetKey} label={`session:${resetKey}`}>
        <MessageStream
          messages={messages}
          ctx={ctx}
          resetKey={resetKey}
          onControlsChange={handleControls}
        />
      </ChatErrorBoundary>
      <div className="pointer-events-none absolute inset-x-0 bottom-0 px-6 pb-4">
        <div
          className="pointer-events-none absolute inset-x-0 bottom-0 h-[140px]"
          style={{
            background: "linear-gradient(180deg, transparent 0%, var(--color-bg) 52%)",
          }}
        />
        <JumpToBottomButton
          visible={streamControls ? !streamControls.isAtBottom : false}
          onClick={() => streamControls?.scrollToBottom()}
        />
        {/* px-5 mirrors msg-stream's content padding so the composer's
            outer edge lines up with the message text column above it. */}
        <div className="pointer-events-auto relative z-[2] mx-auto w-full max-w-[800px] px-5">
          {composer}
        </div>
      </div>
    </>
  );
}
