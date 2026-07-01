import type { BlockCtx } from "../message";
import type { Message } from "@/plugins/builtin/agent/public/viewState";
import { AnimatePresence, motion } from "motion/react";
import { Fragment, useCallback, useEffect } from "react";
import { StickToBottom, useStickToBottomContext } from "use-stick-to-bottom";
import { enterUp } from "@/lib/motion";
import { bcp47 } from "@/lib/i18n/relativeTime";
import { useT } from "@/lib/i18n";
import { Slot } from "@/plugins/host/Slot";
import { useIsAgentRunning } from "@/plugins/builtin/agent/public/run";
import { MessageBlock } from "../message";

// Chat scroll surface, backed by use-stick-to-bottom. `resetKey`
// re-keys the subtree on session switch so a new thread lands at the
// bottom. That landing is `initial="instant"` (a jump, not an animation):
// a smooth initial replays a visible top→bottom scroll through the whole
// history on every mount / session switch / remount — which reads as the
// chat "auto-scrolling on open" and flashes content-visibility gaps as it
// flies past unrendered messages. Only the resize catch-up below stays
// smooth. Follow state surfaces to the parent via ControlsRelay below.

export interface StreamControls {
  isAtBottom: boolean;
  scrollToBottom: () => void;
}

interface Props {
  messages: Message[];
  ctx: BlockCtx;
  /** Re-key on change to reset scroll position + follow state. */
  resetKey: string;
  /** Receives the latest controls; called on any change. ChatPanel
   *  forwards these to the JumpToBottomButton sibling. Pass a stable
   *  reference (e.g. a setState setter) — we call it from an effect. */
  onControlsChange?: (controls: StreamControls) => void;
}

// Bridges StickToBottom's context out of the provider so ChatPanel can
// render the jump-to-bottom button as a sibling (the button needs to
// sit over the composer, not inside the scroll viewport).
function ControlsRelay({ onChange }: { onChange?: (c: StreamControls) => void }) {
  const ctx = useStickToBottomContext();
  // useCallback so the ref handed to consumers stays stable across
  // unrelated re-renders.
  const scrollToBottom = useCallback(() => {
    void ctx.scrollToBottom();
  }, [ctx]);

  useEffect(() => {
    onChange?.({ isAtBottom: ctx.isAtBottom, scrollToBottom });
  }, [ctx.isAtBottom, scrollToBottom, onChange]);

  return null;
}

function formatTurnTime(iso: string | undefined): string {
  if (!iso) return "";
  const d = new Date(iso);
  if (Number.isNaN(d.getTime())) return "";

  const now = new Date();
  const isThisYear = d.getFullYear() === now.getFullYear();

  const opts: Intl.DateTimeFormatOptions = {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    hour12: false,
  };
  if (!isThisYear) opts.year = "numeric";

  return new Intl.DateTimeFormat(bcp47(), opts).format(d);
}

function TurnSeparator({ createdAt }: { createdAt?: string }) {
  // useT() keeps this reactive on locale toggle even though the
  // translation function itself isn't used for the timestamp label.
  useT();
  const label = formatTurnTime(createdAt);
  if (!label) return null;
  return (
    <div className="my-4 text-center text-[12px] text-fg-faint" data-slot="turn-separator">
      {label}
    </div>
  );
}

export function MessageStream({ messages, ctx, resetKey, onControlsChange }: Props) {
  // While a run streams, content grows continuously; the default `resize`
  // spring (stiffness 0.05 / mass 1.25) is too sluggish to track it and the
  // tail scrolls out of view (D2). Hard-pin to the bottom during generation,
  // and keep the smooth catch-up only when idle (re-open / history load).
  // `running` flips only at run boundaries, so this never churns per token.
  const running = useIsAgentRunning();

  const firstUserIndex = messages.findIndex((m) => m.role === "user");

  if (messages.length === 0) {
    return (
      <StickToBottom key={resetKey} className="msg-scroll-frame" initial="instant" resize="smooth">
        <StickToBottom.Content
          scrollClassName="panel-scroll"
          className="relative mx-auto flex w-full max-w-[840px] flex-col gap-7 px-5 pt-8 pb-[220px]"
        >
          <Slot name="chat.empty" />
        </StickToBottom.Content>
        <ControlsRelay onChange={onControlsChange} />
      </StickToBottom>
    );
  }

  return (
    <StickToBottom
      key={resetKey}
      className="panel-scroll msg-scroll"
      initial="instant"
      resize={running ? "instant" : "smooth"}
    >
      <StickToBottom.Content
        scrollClassName="panel-scroll"
        className="relative mx-auto flex w-full max-w-[840px] flex-col gap-7 px-5 pt-8 pb-[220px]"
      >
        <AnimatePresence initial={false}>
          {messages.map((m, i) => (
            <Fragment key={m.id}>
              {m.role === "user" && i !== firstUserIndex && (
                <TurnSeparator createdAt={m.createdAt} />
              )}
              {/* No `layout` prop — Motion's layout animation re-tweens
                  the block on every text delta, making the whole bubble
                  (avatar included) bobble while streaming. enterUp is
                  enough: first paint slides in, then the block grows
                  naturally with the DOM.

                  `content-visibility:auto` lets the browser skip layout+paint for
                  off-screen messages (the long-conversation scaling cliff) while
                  keeping every node IN the DOM — so ⌘F's TreeWalker + CSS-highlight
                  search, copy-all, and stick-to-bottom's height all still work
                  (true virtualization would unmount nodes and break those). The
                  `auto` intrinsic-size remembers each message's real height after
                  its first render, so the scroll height stays accurate; the 220px
                  fallback only covers never-yet-rendered messages far below. */}
              <motion.div
                {...enterUp}
                className="[content-visibility:auto] [contain-intrinsic-size:auto_220px]"
              >
                <MessageBlock msg={m} ctx={ctx} />
              </motion.div>
            </Fragment>
          ))}
        </AnimatePresence>
      </StickToBottom.Content>
      <ControlsRelay onChange={onControlsChange} />
    </StickToBottom>
  );
}
