import { publishStreamFollow } from "./streamFollow";
import type { BlockCtx } from "@/plugins/builtin/chat/message/public/rendering";
import type { TranscriptRow } from "@/plugins/builtin/agent/public/conversation";
import type { Message } from "@/plugins/builtin/agent/public/viewState";
import { AnimatePresence, motion } from "motion/react";
import { memo, useEffect, useImperativeHandle, useRef, type Ref } from "react";
import {
  StickToBottom,
  useStickToBottomContext,
  type StickToBottomContext,
} from "use-stick-to-bottom";
import { enterUp } from "@/lib/motion";
import { useMotionOff } from "@/lib/appearance";
import { cn } from "@/lib/classNames";
import { dayKey, formatDay } from "@/lib/i18n/relativeTime";
import { useT } from "@/lib/i18n";
import { Divider, Loader } from "@/ui";
import { COMPOSER_CLEARANCE, READING_COLUMN, READING_GUTTER } from "./readingColumn";
import { useIsCurrentRootRunning } from "@/plugins/builtin/agent/public/run";
import { MessageBlock, RootRunOutcome } from "@/plugins/builtin/chat/message/public/rendering";
import { transcriptTurnContentVisibility } from "./transcriptTurnContentVisibility";

// Chat scroll surface, backed by use-stick-to-bottom. `resetKey`
// re-keys the subtree on session switch so a new thread lands at the
// bottom. That landing is `initial="instant"` (a jump, not an animation):
// a smooth initial replays a visible top→bottom scroll through the whole
// history on every mount / session switch / remount — which reads as the
// chat "auto-scrolling on open" and flashes content-visibility gaps as it
// flies past unrendered messages. Only the resize catch-up below stays
// smooth. Follow state is published by ControlsRelay below.

interface Props {
  rows: readonly TranscriptRow[];
  ctx: BlockCtx;
  /** Re-key on change to reset scroll position + follow state. */
  resetKey: string;
  controllerRef?: Ref<MessageStreamController>;
}

export interface MessageStreamController {
  /** Reconcile geometry that becomes known during the parent's first layout. */
  settleInitialBottom(): void;
}

const INITIAL_BOTTOM_SETTLE_MS = 250;

// Publishes StickToBottom's follow state out of the provider, for the
// jump-to-bottom button — which has to be a sibling of the scroller to sit over
// the composer, so it cannot read the context itself.
//
// Publishing rather than calling a parent's setState: the context object is rebuilt
// on every scroll event, so reporting it upward re-rendered the component that owns
// the composer at scroll frequency (see streamFollow.ts).
function ControlsRelay() {
  const ctx = useStickToBottomContext();
  // In an effect, not during render: the publish notifies subscribers, and doing
  // that mid-render would be updating one component while another is rendering. No
  // dep array — this runs on each of this (null-rendering) component's renders, so
  // the click handler it hands out is never a stale closure.
  useEffect(() => {
    publishStreamFollow({
      atBottom: ctx.isAtBottom,
      scrollToBottom: () => void ctx.scrollToBottom(),
    });
  });
  return null;
}

// A date, once, where the date changes. The clock time lives in every turn's own
// caption now, so a separator that repeated the full stamp above each turn was
// saying the same thing twice and saying it loudest at the boundary that carried
// the least information.
function DaySeparator({ createdAt }: { createdAt?: string }) {
  // useT() keeps this reactive on locale toggle even though the
  // translation function itself isn't used for the timestamp label.
  useT();
  const label = formatDay(createdAt);
  if (!label) return null;
  return (
    <div className={cn(READING_GUTTER, "py-1")}>
      <Divider align="start">{label}</Divider>
    </div>
  );
}

/**
 * The calendar day a turn falls on, remembered against the turn itself.
 *
 * `dayKey` parses a timestamp, and the day rule needs one per turn on every render of
 * the list — which is every token of a live run. At a few hundred turns that was a few
 * hundred date parses per frame, growing with the conversation for a grouping that
 * cannot change: a turn's timestamp is fixed once the fold has written it. Keyed on the
 * message object so an entry is collectable the moment the fold replaces it.
 */
const dayKeyByMessage = new WeakMap<Message, string | null>();

function turnDayKey(message: Message): string | null {
  const cached = dayKeyByMessage.get(message);
  if (cached !== undefined) return cached;
  const key = dayKey(message.createdAt);
  dayKeyByMessage.set(message, key);
  return key;
}

/** Two distances, not one. A flat gap made a turn's own blocks sit as far apart as two
 *  separate turns, so nothing on the page said "these belong together" — the reference
 *  spends 4px inside a turn and 16px between them, and that ratio is what groups a
 *  thought, its tool call and its answer into one thing you can read as a unit. */
const TURN_GAP = {
  none: "",
  sameSpeaker: "mt-1",
  newSpeaker: "mt-4",
} as const;

interface TurnProps {
  row: TranscriptRow;
  ctx: BlockCtx;
  isLast: boolean;
  isRunning: boolean;
  /** A new calendar day starts here. Decided by the list, because it is a relationship
   *  between two turns and no turn can see the one above it. */
  opensDay: boolean;
  gap: keyof typeof TURN_GAP;
}

/**
 * One turn in the transcript, with the chrome that positions it.
 *
 * Its own component, and memoised, because the list re-renders on every token of a live
 * run: without this boundary React re-rendered every row's `motion.div` on every delta
 * — Motion components are not memoised, so the wrapper did its full prop diff and
 * visual-element update several hundred times a frame while the content inside it (which
 * WAS memoised) bailed out. Every prop here is a primitive or a reference the transcript
 * projection keeps stable, so an untouched turn costs nothing.
 */
const TranscriptTurn = memo(function TranscriptTurn({
  row,
  ctx,
  isLast,
  isRunning,
  opensDay,
  gap,
}: TurnProps) {
  return (
    <>
      {opensDay && <DaySeparator createdAt={row.message.createdAt} />}
      {/* No `layout` prop — Motion's layout animation re-tweens the block on every
          text delta, making the whole bubble (avatar included) bobble while streaming.
          enterUp is enough: first paint slides in, then the block grows naturally with
          the DOM.

          `content-visibility:auto` lets the browser skip layout+paint for off-screen
          messages (the long-conversation scaling cliff) while keeping every node IN the
          DOM — so ⌘F's TreeWalker + CSS-highlight search, copy-all, and
          stick-to-bottom's height all still work (true virtualization would unmount
          nodes and break those). The `auto` intrinsic-size remembers each message's
          real height after its first render, so the scroll height stays accurate; the
          220px fallback only covers never-yet-rendered messages far below. */}
      {/* `data-turn-id` is the anchor the narrative rails navigate by. An attribute
          rather than a registry: the rails need the element's position in the scroller,
          which only the DOM has. */}
      <motion.div
        {...enterUp}
        data-turn-id={row.message.id}
        data-turn-role={row.message.role}
        // The gutter lives HERE, not on the scroller's content, and that is what gives
        // this box slack on either side of its own text. `content-visibility` brings
        // paint containment with it: anything drawn past this element's edge is
        // clipped, and the turn's last row is a strip of round buttons deliberately
        // inset outward so their glyphs line up with the text. Against a box that
        // hugged the text they came out sliced.
        className={cn(READING_GUTTER, TURN_GAP[gap], transcriptTurnContentVisibility(isLast))}
      >
        <MessageBlock row={row} ctx={ctx} isLast={isLast} isRunning={isRunning} />
      </motion.div>
    </>
  );
});

export function MessageStream({ rows, ctx, resetKey, controllerRef }: Props) {
  // While a run streams, content grows continuously; the default `resize`
  // spring (stiffness 0.05 / mass 1.25) is too sluggish to track it and the
  // tail scrolls out of view (D2). Hard-pin to the bottom during generation,
  // and keep the smooth catch-up only when idle (re-open / history load).
  // `running` flips only at run boundaries, so this never churns per token.
  const running = useIsCurrentRootRunning();
  const motionOff = useMotionOff();
  const stickContextRef = useRef<StickToBottomContext>(null);
  const initialSettleFrameRef = useRef<number | null>(null);

  useEffect(
    () => () => {
      if (initialSettleFrameRef.current !== null) {
        cancelAnimationFrame(initialSettleFrameRef.current);
      }
    },
    [],
  );

  // Keep the scroll library behind MessageStream's local controller. The
  // parent owns shared composer/transcript geometry, but it should not know
  // which package implements transcript following. `scrollToBottom` performs
  // its write on the next frame, after the parent's custom-property update has
  // been included in the content height.
  useImperativeHandle(
    controllerRef,
    () => ({
      settleInitialBottom() {
        const stickContext = stickContextRef.current;
        const viewport = stickContext?.scrollRef.current;
        if (!viewport) return;

        if (initialSettleFrameRef.current !== null) {
          cancelAnimationFrame(initialSettleFrameRef.current);
        }

        const settleUntil = performance.now() + INITIAL_BOTTOM_SETTLE_MS;
        const pinCurrentBottom = () => {
          const currentViewport = stickContextRef.current?.scrollRef.current;
          if (!currentViewport) {
            initialSettleFrameRef.current = null;
            return;
          }

          // Reading scrollHeight commits the parent's newly-published composer
          // clearance. Pinning the real DOM value for this bounded mount window
          // also covers Markdown and syntax highlighting that resolve on an
          // adjacent frame without letting a library animation retain a stale
          // target. After the window, the library regains normal escape rules.
          currentViewport.scrollTop = currentViewport.scrollHeight;
          initialSettleFrameRef.current =
            performance.now() < settleUntil ? requestAnimationFrame(pinCurrentBottom) : null;
        };

        pinCurrentBottom();
        void stickContext?.scrollToBottom({
          animation: "instant",
          ignoreEscapes: true,
        });
      },
    }),
    [],
  );

  // No empty branch: the only caller mounts this once a transcript exists, and the
  // empty home is its own layout (centred, no scroller). The branch that used to be
  // here rendered a second copy of the `chat.empty` slot inside a stick-to-bottom
  // scroller that nothing could ever scroll.

  // The last day seen, carried across the map below. A turn with no timestamp neither
  // opens a day nor breaks the chain — absent is not a different day, it is no
  // information, and letting it reset this put a separator above the next turn that had
  // one.
  let previousDay: string | null = null;

  return (
    <StickToBottom
      key={resetKey}
      contextRef={stickContextRef}
      className="panel-scroll msg-scroll"
      initial="instant"
      // A transcript that eases itself into place is motion, and motion is a
      // preference. The scroll library has no idea the user turned it off, so
      // the one place that knows tells it — otherwise "reduce motion" leaves
      // the one surface that moves most still moving.
      resize={running || motionOff ? "instant" : "smooth"}
    >
      {/* `msg-scroll-viewport` names the element that actually scrolls. The
          library renders it itself, one level inside the class above, so anything
          outside the transcript that needs the scroll box — the narrative rails —
          would otherwise have to guess at that nesting. */}
      <StickToBottom.Content
        scrollClassName="panel-scroll msg-scroll-viewport"
        className={cn(READING_COLUMN, COMPOSER_CLEARANCE, "relative flex flex-col pt-8")}
      >
        <AnimatePresence initial={false}>
          {rows.map((row, index) => {
            const day = turnDayKey(row.message);
            const opensDay = day !== null && previousDay !== null && day !== previousDay;
            if (day !== null) previousDay = day;
            const previousRole = index > 0 ? rows[index - 1]?.message.role : undefined;
            return (
              <TranscriptTurn
                key={row.message.id}
                row={row}
                ctx={ctx}
                isLast={index === rows.length - 1}
                isRunning={running}
                opensDay={opensDay}
                gap={
                  previousRole === undefined
                    ? "none"
                    : previousRole === row.message.role
                      ? "sameSpeaker"
                      : "newSpeaker"
                }
              />
            );
          })}
        </AnimatePresence>
        {/* Waiting-for-response indicator — the run is live but the assistant
            hasn't opened its turn yet (last message is still the user's). Once
            the assistant message arrives it takes over, so this hides itself. */}
        {running && rows[rows.length - 1]?.message.role === "user" && (
          <div className={cn(READING_GUTTER, "mt-4 flex")}>
            <Loader variant="dots" />
          </div>
        )}
        {!running && (
          <div className={cn(READING_GUTTER, "mt-4")}>
            <RootRunOutcome />
          </div>
        )}
      </StickToBottom.Content>
      <ControlsRelay />
    </StickToBottom>
  );
}
