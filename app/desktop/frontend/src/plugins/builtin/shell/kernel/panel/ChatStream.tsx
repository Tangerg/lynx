// ChatStream — the transcript, its banners, and where the composer sits.
//
// Owns the agent / session slices it actually reads (no fat shared interface) and
// the auto-select-latest-tool effect. It deliberately holds NEITHER of the two
// high-frequency states around it: the composer's draft lives in ComposerSurface
// and the scroll follow state in streamFollow, because a component that renders the
// message list must not re-render on every keystroke or every scroll event.

import type { UserInput } from "@/plugins/builtin/chat/composer/public/input";
import { useEffect, useLayoutEffect, useMemo, useRef } from "react";
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
import { READING_COLUMN, READING_GUTTER } from "./readingColumn";
import { CwdMissingBanner } from "./CwdMissingBanner";
import { JumpToBottomButton } from "./JumpToBottomButton";
import { MessageStream } from "./MessageStream";
import { RunErrorBanner } from "./RunErrorBanner";

interface Props {
  /** Send the user's message input (text + inlined images) through the live agent. */
  onSend: (input: UserInput) => void;
}

// The two rails hang off that column's edges, OUT of its flow.
//
// They used to be flex siblings, and the column sat at the midpoint of the two
// gutters instead of the pane's — 76px left of centre, because the outline rail
// is wider than the turn rail. Reserving the gutters was itself a repair: a rail
// that collapsed when its turn had no outline used to drag the transcript
// sideways mid-scroll. Positioned absolutely against the centre line, neither
// failure is reachable — a rail can appear, disappear or change width and the
// text does not move, because the text's position never depended on it.
//
// One query for both, since the pane must hold the column and a FULL rail on
// each side to stay symmetric. The sum (720 + 2×224) is spelled out because
// Tailwind reads source text and a variant built from a variable emits nothing.
//
// Transparent to the pointer, and only what a rail actually draws takes it back:
// the scroller underneath now spans the whole pane, and a 224px pad of nothing
// that swallows the wheel is a pane that stops scrolling wherever you happen to
// have left the cursor. Bounded above the composer for the same reason the
// transcript pads its tail — the overlay is opaque, and a list that runs under
// it is a list with items nobody can reach.
const RAIL =
  "absolute top-0 bottom-[var(--composer-overlay,0px)] z-[1] hidden w-[var(--reading-rail-width)] flex-col @min-[1168px]:flex pointer-events-none [&>*]:pointer-events-auto";
const RAIL_START = "right-[calc(50%+var(--reading-column-max)/2)]";
const RAIL_END = "left-[calc(50%+var(--reading-column-max)/2)]";

// How much of itself the floating composer hides. The transcript pads its tail
// by this so the last message can always come out from under it — measured,
// because the composer grows with what you type and with whatever the model row,
// the attachments and the steer banner are showing.
const COMPOSER_OVERLAY = "--composer-overlay";

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
  const overlayRef = useRef<HTMLDivElement>(null);
  // Written straight to the element, not held in state: the composer resizes on
  // the keystroke that wraps a line, and routing that through a render would put
  // the whole message list on the typing path — the one thing this component is
  // organised to keep it off.
  useLayoutEffect(() => {
    const pane = paneRef.current;
    const overlay = overlayRef.current;
    if (!pane || !overlay) return;
    const observer = new ResizeObserver(() => {
      pane.style.setProperty(COMPOSER_OVERLAY, `${overlay.offsetHeight}px`);
    });
    observer.observe(overlay);
    return () => observer.disconnect();
    // The empty state is a different tree with no overlay in it, so the
    // observer has to be re-attached to the one the first message brings.
  }, [started]);

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

  // Empty state is a workbench starting point rather than a marketing hero. It
  // stays in the upper reading field so the first action is visible without a
  // large dead canvas, and the sticky-scroll path mounts only after first send.
  if (!started) {
    return (
      <>
        {banners}
        <div className="panel-scroll flex flex-1 flex-col items-center pt-[clamp(72px,16vh,150px)]">
          <div className={cn(READING_COLUMN, READING_GUTTER, "flex flex-col pb-5")}>
            <h1 className="max-w-[620px] text-balance text-display-md font-medium text-fg/95">
              {t("welcome.title")}
            </h1>
          </div>
          <div className={cn(READING_COLUMN, READING_GUTTER)}>{composer}</div>
          <div className={cn(READING_COLUMN, READING_GUTTER, "mt-6")}>
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
        <div className={cn(RAIL, RAIL_START)}>
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
        <div className={cn(RAIL, RAIL_END)}>
          <Slot name="chat.rail.end" />
        </div>

        {/* The composer floats over the tail of the transcript rather than
            capping it with a bar: one continuous surface with an input resting
            on it, which is also why the text has to keep going underneath. The
            band above it fades that text out instead of slicing it, and only
            the composer's own box takes the pointer — the fade is scenery.

            It is exactly the COLUMN wide, never the pane. A full-width overlay
            is a bottom bar however it is positioned: it paints over the whole
            width of the pane, which reads as chrome and takes the scrollbar's
            bottom inch with it. Nothing outside the column has anything to hide
            anyway — the transcript is centred and capped. */}
        <div
          ref={overlayRef}
          className={cn("pointer-events-none absolute inset-x-0 bottom-0 z-10", READING_COLUMN)}
        >
          <div
            className={cn(
              READING_GUTTER,
              "h-8 bg-gradient-to-b from-transparent to-[var(--app-content-surface)]",
            )}
          />
          <div className={cn(READING_GUTTER, "bg-[var(--app-content-surface)] pb-3 sm:pb-4")}>
            <div className="pointer-events-auto relative">
              <JumpToBottomButton />
              {composer}
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}
