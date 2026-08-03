// ChatStream — the transcript, its banners, and where the composer sits.
//
// Owns the agent / session slices it actually reads (no fat shared interface) and
// the auto-select-latest-tool effect. It deliberately holds NEITHER of the two
// high-frequency states around it: the composer's draft lives in ComposerSurface
// and the scroll follow state in streamFollow, because a component that renders the
// message list must not re-render on every keystroke or every scroll event.

import type { UserInput } from "@/plugins/builtin/chat/composer/public/input";
import { useEffect, useMemo, useRef } from "react";
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
import { FloatingComposer } from "./FloatingComposer";
import { READING_COLUMN, READING_GUTTER } from "./readingColumn";
import { CwdMissingBanner } from "./CwdMissingBanner";
import { MessageStream } from "./MessageStream";
import { RunErrorBanner } from "./RunErrorBanner";

interface Props {
  /** Send the user's message input (text + inlined images) through the live agent. */
  onSend: (input: UserInput) => void;
}

// The turn map hangs off the reading column's leading edge, OUT of its flow.
//
// It used to be a flex sibling, and the column sat at the midpoint of the two
// gutters instead of the pane's. Reserving those gutters was itself a repair: a
// rail that collapsed when its turn had nothing to show used to drag the
// transcript sideways mid-scroll. Positioned absolutely against the centre line,
// neither failure is reachable — the rail can appear, disappear or change width
// and the text does not move, because the text's position never depended on it.
//
// The query keeps the layout symmetric even though only one side carries a rail:
// the pane must hold the column and a full gutter EITHER side, or the map ends
// up crowding an off-centre column. The sum (800 + 2×176) is spelled out because
// Tailwind reads source text and a variant built from a variable emits nothing.
//
// Transparent to the pointer, and only what the rail draws takes it back: the
// scroller underneath spans the whole pane, and a pad of nothing that swallows
// the wheel is a pane that stops scrolling wherever you left the cursor.
// Bounded above the composer for the same reason the transcript pads its tail —
// the overlay is opaque, and marks that run under it are marks nobody can hit.
const RAIL =
  "absolute top-0 bottom-[var(--composer-overlay,0px)] z-[1] hidden w-[var(--reading-rail-width)] flex-col @min-[1152px]:flex pointer-events-none [&>*]:pointer-events-auto right-[calc(50%+var(--reading-column-max)/2)]";

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
  const started = messages.length > 0;

  const paneRef = useRef<HTMLDivElement>(null);

  const t = useT();

  // Pinned above whatever fills the column — a session with a hundred turns or a
  // brand-new one. Both states carry them: the goal control lives in this slot,
  // and rendering it only once a conversation had started meant the affordance was
  // missing at exactly the moment you set an objective.
  //
  // The stream's scroll lives inside MessageStream's own container, so these stay
  // put while the user scrolls messages below them.
  const banners = (
    <div className={cn(READING_COLUMN, READING_GUTTER, "shrink-0")}>
      {/* Keyed on the session so the relocate input never carries a
          half-typed path across a session switch. */}
      <CwdMissingBanner key={resetKey} />
      <RunErrorBanner />
      <div className="pointer-events-auto">
        <Slot name="chat.banner.top" />
      </div>
    </div>
  );

  // Empty state: the question and the place to answer it, centred, and nothing
  // else. It carried a row of suggestion pills and a strip of keyboard hints, which
  // is a marketing hero pretending to be onboarding — the pills wrote someone
  // else's sentence into your input, and the hints put shortcut glyphs on the first
  // screen of a workbench.
  //
  // Centred rather than pinned to the upper field: with the ornament gone there is
  // nothing for a top-weighted stack to hold together, and two elements floating a
  // third of the way down read as a page that failed to load.
  //
  // The slot stays because one contribution earns it — an install with no provider
  // key cannot send anything, so the way to fix that has to be here. It renders
  // nothing otherwise.
  if (!started) {
    return (
      <>
        {banners}
        <div className="panel-scroll flex flex-1 flex-col items-center justify-center gap-5 pb-[6vh]">
          {/* Centred over the input rather than flush with the column's text edge:
              the input insets its own placeholder, so a left-aligned title started
              14px before the words underneath it and read as a near-miss. */}
          <div className={cn(READING_COLUMN, READING_GUTTER)}>
            <h1 className="mx-auto max-w-[620px] text-balance text-center text-display-md font-medium text-fg">
              {t("welcome.title")}
            </h1>
          </div>
          <div className={cn(READING_COLUMN, READING_GUTTER)}>{composer}</div>
          <div className={cn(READING_COLUMN, READING_GUTTER, "empty:hidden")}>
            <Slot name="chat.empty" />
          </div>
        </div>
      </>
    );
  }

  return (
    // A container query and not a viewport one: what decides whether a rail fits
    // is the width of THIS pane, which the drawer and the dock both change
    // without the window changing at all.
    <div ref={paneRef} className="@container relative flex min-h-0 flex-1 flex-col">
      {banners}
      <div className="relative flex min-h-0 flex-1 flex-col">
        <div className={RAIL}>
          <Slot name="chat.rail.start" />
        </div>
        {/* The SCROLLER is the pane, not the column — the column is centred
            inside it. Scrolling a 680px box puts its scrollbar 680px in, right
            down the edge of the text; the pane's own edge is where every other
            application puts it and the only place it isn't in the way. */}
        <div className="relative flex min-h-0 flex-1 flex-col">
          <ChatErrorBoundary resetKey={resetKey} label={`session:${resetKey}`}>
            <MessageStream messages={messages} ctx={ctx} resetKey={resetKey} />
          </ChatErrorBoundary>
        </div>

        <FloatingComposer publishHeightTo={paneRef}>{composer}</FloatingComposer>
      </div>
    </div>
  );
}
