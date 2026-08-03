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
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { Slot } from "@/plugins/host/Slot";
import {
  selectInitialWorkspaceTool,
  useExpandedWorkspaceToolIds,
  useSelectWorkspaceTool,
  useToggleWorkspaceTool,
} from "@/plugins/builtin/workspace/public/navigation";
import { useUiStore } from "@/state/uiStore";
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

// The reading column's two flanking gutters.
//
// The COLUMN reserves them, not the rails. A rail with nothing to show still
// holds its width — a gutter that collapses when the current turn has no outline
// slides the whole transcript sideways as you scroll. It also means the rails'
// own components carry no breakpoint: where the column can afford a gutter is
// the column's question, and answering it in two places is how the banners, the
// stream and the composer ended up centred on three different boxes.
//
// The breakpoints are literals because Tailwind reads source text and a
// container-query variant assembled from a variable emits nothing; the widths
// are tokens because they are geometry the theme owns.
const RAIL_START = "w-0 shrink-0 overflow-hidden @min-[560px]:w-[var(--rail-start-width)]";
const RAIL_END = "w-0 shrink-0 overflow-hidden @min-[900px]:w-[var(--rail-end-width)]";
const RAIL_GUTTERS =
  "@min-[560px]:pl-[var(--rail-start-width)] @min-[900px]:pr-[var(--rail-end-width)]";

// Column + gutters centre as ONE block, so a rail never drifts away from the text
// it points at when the pane is wider than the frame.
const READING_FRAME = "mx-auto w-full max-w-[var(--reading-frame-max)]";

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

  // Empty state is a workbench starting point rather than a marketing hero. It
  // stays in the upper reading field so the first action is visible without a
  // large dead canvas, and the sticky-scroll path mounts only after first send.
  if (messages.length === 0) {
    return (
      <>
        {banners}
        <div className="panel-scroll flex flex-1 flex-col items-center px-[var(--density-column-gutter)] pt-[clamp(72px,16vh,150px)] sm:px-[var(--density-column-gutter-wide)]">
          <div className="flex w-full max-w-[var(--content-max)] flex-col pb-5">
            <h1 className="max-w-[620px] text-balance text-display-md font-medium text-fg/95">
              {t("welcome.title")}
            </h1>
          </div>
          <div className="w-full max-w-[var(--content-max)]">{composer}</div>
          <div className="mt-6 w-full max-w-[var(--content-max)]">
            <Slot name="chat.empty" />
          </div>
        </div>
      </>
    );
  }

  return (
    // One container for the whole column. A container query and not a viewport
    // one: what decides whether a rail fits is the width of THIS column, which
    // the drawer and the dock both change without the window changing at all.
    // Banners and composer take the rails' gutters from the same query, so the
    // three stay on one axis instead of each centring on a different box.
    <div className="@container flex min-h-0 flex-1 flex-col">
      <div className={cn(READING_FRAME, RAIL_GUTTERS)}>{banners}</div>
      <div className={cn("relative flex min-h-0 flex-1", READING_FRAME)}>
        <div className={cn("flex flex-col", RAIL_START)}>
          <Slot name="chat.rail.start" />
        </div>
        <div className="relative flex min-h-0 min-w-0 flex-1 flex-col">
          <ChatErrorBoundary resetKey={resetKey} label={`session:${resetKey}`}>
            <MessageStream messages={messages} ctx={ctx} resetKey={resetKey} />
          </ChatErrorBoundary>
          <JumpToBottomButton />
        </div>
        <div className={cn("flex flex-col", RAIL_END)}>
          <Slot name="chat.rail.end" />
        </div>
      </div>
      <div
        className={cn(
          "relative z-10 shrink-0 bg-[var(--app-content-surface)] px-3 pb-3 pt-2 sm:px-5 sm:pb-4",
          READING_FRAME,
          RAIL_GUTTERS,
        )}
      >
        <div className="mx-auto w-full max-w-[var(--content-max)]">{composer}</div>
      </div>
    </div>
  );
}
