// ChatStream — the transcript, its banners, and where the composer sits.
//
// Owns the agent / session slices it actually reads (no fat shared interface) and
// the auto-select-latest-tool effect. It deliberately holds NEITHER of the two
// high-frequency states around it: the composer's draft lives in ComposerSurface
// and the scroll follow state in streamFollow, because a component that renders the
// message list must not re-render on every keystroke or every scroll event.

import type { UserInput } from "@/plugins/builtin/chat/composer/public/input";
import { useEffect, useLayoutEffect, useMemo, useRef } from "react";
import { useActiveConversationRows } from "@/plugins/builtin/agent/public/conversation";
import { useActiveSessionToolCalls } from "@/plugins/builtin/agent/public/run";
import { useActiveSessionId } from "@/plugins/builtin/agent/public/session";
import { cn } from "@/lib/classNames";
import { Slot } from "@/plugins/host/Slot";
import {
  reconcileWorkspaceToolSelection,
  useExpandedWorkspaceToolIds,
  useSelectWorkspaceTool,
  useToggleWorkspaceTool,
} from "@/plugins/builtin/workspace/public/navigation";
import { useUiStore } from "@/state/uiStore";
import { ChatErrorBoundary } from "./ChatErrorBoundary";
import { ComposerSurface } from "./ComposerSurface";
import { ComposerOverlayTop, FloatingComposer, RuntimeConnectionNotice } from "./FloatingComposer";
import { COMPOSER_OVERLAY_PROPERTY, READING_COLUMN, READING_GUTTER } from "./readingColumn";
import { CwdMissingBanner } from "./CwdMissingBanner";
import { MessageStream, type MessageStreamController } from "./MessageStream";
import { RunErrorBanner } from "./RunErrorBanner";
import { EmptyChatHeading } from "./ProjectSelector";
import {
  pendingQuestionRequest,
  QuestionCard,
} from "@/plugins/builtin/chat/message/public/rendering";

interface Props {
  /** Send the user's message input (text + inlined images) through the live agent. */
  onSend: (input: UserInput) => boolean;
}

// The turn map hangs off the reading column's leading edge, OUT of its flow.
//
// Positioned absolutely against the centre line,
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
  "absolute top-0 bottom-[var(--composer-overlay,0px)] z-1 hidden w-[var(--reading-rail-width)] flex-col @min-[1152px]:flex pointer-events-none [&>*]:pointer-events-auto right-[calc(50%+var(--reading-column-max)/2)]";

export function ChatStream({ onSend }: Props) {
  const sessionId = useActiveSessionId();
  const rows = useActiveConversationRows();
  const toolCalls = useActiveSessionToolCalls();

  const expandedToolIds = useExpandedWorkspaceToolIds();
  const selectTool = useSelectWorkspaceTool();
  const toggleExpandedTool = useToggleWorkspaceTool();

  // Global streaming-reveal preference. Read once here (stable string) and
  // threaded through ctx so MarkdownMessage stays prop-driven — no per-block
  // store subscription on the hot streaming path.
  const textReveal = useUiStore((state) => state.streamReveal);

  // Keep the target exact across authoritative snapshot replacement. A user's
  // surviving selection stays put; if compaction/recovery removes it, the
  // latest surviving tool takes ownership without expanding the inline card.
  // Output deltas replace the Tool map too, so the membership signature keeps
  // this effect off that hot path while still changing when compaction removes
  // an id without changing the latest surviving id.
  const toolIdSignature = useMemo(() => Object.keys(toolCalls).join("\u001f"), [toolCalls]);
  const toolIds = useMemo(
    () => (toolIdSignature ? toolIdSignature.split("\u001f") : []),
    [toolIdSignature],
  );
  useEffect(() => {
    reconcileWorkspaceToolSelection(toolIds);
  }, [toolIds]);

  // The transcript's shared context, and every member of it is session-independent by
  // construction — see BlockCtx. That is what makes the memo on each turn able to bail:
  // this object survives a whole run untouched, so a streaming token changes nothing
  // except the one row it belongs to. Session facts reach a turn through its row.
  const ctx = useMemo(
    () => ({
      onSelectTool: selectTool,
      expandedIds: expandedToolIds,
      onToggleExpand: toggleExpandedTool,
      textReveal,
    }),
    [selectTool, expandedToolIds, toggleExpandedTool, textReveal],
  );

  const pendingQuestion = useMemo(() => pendingQuestionRequest(rows), [rows]);
  const composer = pendingQuestion ? (
    <QuestionCard {...pendingQuestion} />
  ) : (
    <ComposerSurface onSend={onSend} />
  );
  const started = rows.length > 0;

  const paneRef = useRef<HTMLDivElement>(null);
  const composerOverlayRef = useRef<HTMLDivElement>(null);
  const messageStreamRef = useRef<MessageStreamController>(null);

  // ChatStream owns both siblings whose geometry is coupled: the transcript
  // consumes the height and the floating composer produces it. Measure in the
  // parent's layout phase, when both refs are attached but before the
  // stick-to-bottom viewport calculates its first target. Publishing from the
  // child deferred the initial value until ResizeObserver and opened every long
  // conversation one composer-height above its tail. Later textarea growth is a
  // direct style write — no transcript render on the typing path.
  useLayoutEffect(() => {
    if (!started) return;
    const pane = paneRef.current;
    const overlay = composerOverlayRef.current;
    if (!pane || !overlay) return;

    const publishHeight = (height = overlay.getBoundingClientRect().height) => {
      // Keep the sub-pixel border-box measurement. `offsetHeight` rounds it to
      // an integer and can round down, leaving the transcript's final baseline
      // one physical pixel inside the glass composer at some scale factors.
      pane.style.setProperty(COMPOSER_OVERLAY_PROPERTY, `${height}px`);
    };

    publishHeight();
    messageStreamRef.current?.settleInitialBottom();
    const observer = new ResizeObserver(([entry]) => {
      const borderBox = entry?.borderBoxSize[0];
      publishHeight(borderBox?.blockSize);
    });
    observer.observe(overlay);
    return () => {
      observer.disconnect();
    };
  }, [started]);

  // Session and run problems stay pinned above whatever fills the column. Standing
  // Goal/Plan material belongs to FloatingComposer's quiet top overlay. Both
  // remain available while the transcript scrolls without adding an inner edge
  // to the composer surface.
  const banners = (
    <div className={cn(READING_COLUMN, READING_GUTTER, "shrink-0")}>
      {/* Keyed on the session so the relocate input never carries a
          half-typed path across a session switch. */}
      <CwdMissingBanner key={sessionId} />
      <RunErrorBanner />
      <Slot
        name="chat.banner.top"
        wrapper
        className="pointer-events-auto flex flex-col gap-1.5 py-1.5"
      />
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
              <EmptyChatHeading />
            </h1>
          </div>
          <div className={cn(READING_COLUMN, READING_GUTTER)}>
            <ComposerOverlayTop />
            <RuntimeConnectionNotice />
            {composer}
          </div>
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
          <ChatErrorBoundary resetKey={sessionId} label={`session:${sessionId}`}>
            <MessageStream
              rows={rows}
              ctx={ctx}
              sessionId={sessionId}
              controllerRef={messageStreamRef}
            />
          </ChatErrorBoundary>
        </div>

        <FloatingComposer overlayRef={composerOverlayRef}>{composer}</FloatingComposer>
      </div>
    </div>
  );
}
