// ChatStream — the transcript, its banners, and where the composer sits.
//
// Owns the agent / session slices it actually reads (no fat shared interface) and
// the auto-select-latest-tool effect. It deliberately holds NEITHER of the two
// high-frequency states around it: the composer's draft lives in ComposerSurface
// and the scroll follow state in streamFollow, because a component that renders the
// message list must not re-render on every keystroke or every scroll event.

import type { UserInput } from "@/plugins/builtin/chat/composer/public/input";
import { useEffect, useMemo } from "react";
import {
  useActiveConversationMessages,
  useDelegatedConversationRuns,
} from "@/plugins/builtin/agent/public/conversation";
import { useActiveSessionToolCalls, useCurrentRootPlan } from "@/plugins/builtin/agent/public/run";
import { useActiveSessionId } from "@/plugins/builtin/agent/public/session";
import { useT } from "@/lib/i18n";
import { Slot } from "@/plugins/host/Slot";
import {
  selectInitialWorkspaceTool,
  useExpandedWorkspaceToolIds,
  useSelectWorkspaceTool,
  useToggleWorkspaceTool,
} from "@/plugins/builtin/workspace/public/navigation";
import { useUiStore } from "@/state/uiStore";
import { Icon } from "@/ui";
import { ChatErrorBoundary } from "./ChatErrorBoundary";
import { ComposerSurface } from "./ComposerSurface";
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
  const delegatedRunsByItemId = useDelegatedConversationRuns();
  const plan = useCurrentRootPlan();
  const toolCalls = useActiveSessionToolCalls();

  const expandedToolIds = useExpandedWorkspaceToolIds();
  const selectTool = useSelectWorkspaceTool();
  const toggleExpandedTool = useToggleWorkspaceTool();

  // Global streaming-reveal preference. Read once here (stable string) and
  // threaded through ctx so MarkdownMessage stays prop-driven — no per-block
  // store subscription on the hot streaming path.
  const typewriter = useUiStore((s) => s.streamReveal) === "typewriter";

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
      delegatedRunsByItemId,
      onSelectTool: selectTool,
      expandedIds: expandedToolIds,
      onToggleExpand: toggleExpandedTool,
      typewriter,
    }),
    [
      plan,
      toolCalls,
      delegatedRunsByItemId,
      selectTool,
      expandedToolIds,
      toggleExpandedTool,
      typewriter,
    ],
  );

  const composer = <ComposerSurface onSend={onSend} />;

  const t = useT();

  // Pinned above whatever fills the column — a session with a hundred turns or a
  // brand-new one. Both states carry them: the goal control lives in this slot,
  // and rendering it only once a conversation had started meant the affordance was
  // missing at exactly the moment you set an objective.
  //
  // The stream's scroll lives inside MessageStream's own container, so these stay
  // put while the user scrolls messages below them.
  const banners = (
    <div className="mx-auto w-full max-w-[var(--content-max)] shrink-0 px-[var(--density-column-gutter)] sm:px-[var(--density-column-gutter-wide)]">
      {/* Keyed on the session so the relocate input never carries a
          half-typed path across a session switch. */}
      <CwdMissingBanner key={resetKey} />
      <RunErrorBanner />
      <div className="pointer-events-auto">
        <Slot name="chat.banner.top" />
      </div>
    </div>
  );

  // Empty state (Codex / ChatGPT voice): the hero + composer are ONE
  // vertically-centered group. No MessageStream / StickToBottom here — nothing
  // is streaming yet, so the delicate sticky-scroll path only mounts once there
  // are messages, and the composer "drops" to the bottom on the first send.
  if (messages.length === 0) {
    return (
      <>
        {banners}
        {/* Hero and composer are ONE vertically-centred group, so the composer
            sits at the optical centre rather than the heading floating above a
            bottom-anchored input. */}
        <div className="panel-scroll flex flex-1 flex-col items-center justify-center px-[var(--density-column-gutter)] sm:px-[var(--density-column-gutter-wide)]">
          <div className="flex w-full max-w-[var(--content-max)] flex-col items-center gap-3 pb-5">
            <Icon name="spark" size="xl" className="text-fg" />
            <h1 className="text-balance text-center text-display-lg font-normal text-fg/95 sm:text-display-xl">
              {t("welcome.title")}
            </h1>
          </div>
          <div className="w-full max-w-[var(--content-max)]">{composer}</div>
          <div className="mt-8 w-full max-w-[var(--content-max)]">
            <Slot name="chat.empty" />
          </div>
        </div>
      </>
    );
  }

  return (
    <>
      {banners}
      {/* The transcript scrolls; the composer is a sibling in normal flow that
          pulls UP over it. That overlap is what makes the composer read as
          floating on the conversation, and it means the scroller needs no
          reserved bottom padding sized to the composer — which was a magic
          number that silently went stale every time the composer grew a row. */}
      <div className="relative flex min-h-0 flex-1 flex-col">
        <ChatErrorBoundary resetKey={resetKey} label={`session:${resetKey}`}>
          <MessageStream messages={messages} ctx={ctx} resetKey={resetKey} />
        </ChatErrorBoundary>
        <JumpToBottomButton />
      </div>
      <div className="relative z-10 -mt-5 w-full shrink-0 overflow-visible px-3 pb-3 sm:px-5 sm:pb-4">
        <div className="mx-auto w-full max-w-[var(--content-max)]">{composer}</div>
      </div>
    </>
  );
}
