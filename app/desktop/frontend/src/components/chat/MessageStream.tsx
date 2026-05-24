import { useCallback, useEffect, type Ref } from "react";
import { AnimatePresence, motion } from "motion/react";
import { StickToBottom, useStickToBottomContext } from "use-stick-to-bottom";
import { enterUp } from "@/lib/motion";
import { Slot } from "@/plugins/Slot";
import type { Message } from "@/protocol/agui/viewState";
import { MessageBlock } from "./MessageBlock";
import type { PartCtx } from "./PartRenderer";

// MessageStream — chat scroll surface, backed by `use-stick-to-bottom`.
//
// Why the library: we tried hand-rolling sticky-bottom-during-stream
// twice (useStickyBottomScroll v1 / v2). Both ran into the same edge
// cases:
//   - macOS trackpad momentum scrolling fires `scroll` events after our
//     200ms user-input window expired, breaking user/programmatic
//     discrimination.
//   - The 220px composer padding means "literal scroll max" is far
//     below "the user perceives they're at the bottom".
//   - Smooth-text reveals that extend an existing line (no height
//     change) don't fire ResizeObserver, so follow stalls between
//     line wraps.
// `use-stick-to-bottom` (the same lib portai uses) handles all three.
// We keep the JumpToBottomButton + composer in ChatPanel, but read
// follow state from the library via the small ControlsRelay below.
//
// `resetKey` re-keys the whole scroll subtree on session switch so a
// fresh thread always lands at its bottom (initial="smooth" runs again).

export type StreamControls = {
  isAtBottom: boolean;
  scrollToBottom: () => void;
};

type Props = {
  messages: Message[];
  ctx: PartCtx;
  /** Re-key on change to reset scroll position + follow state. */
  resetKey: string;
  /** Receives the latest controls; called on any change. ChatPanel
   *  forwards these to the JumpToBottomButton sibling. Pass a stable
   *  reference (e.g. a setState setter) — we call it from an effect. */
  onControlsChange?: (controls: StreamControls) => void;
};

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

export function MessageStream({ messages, ctx, resetKey, onControlsChange }: Props) {
  if (messages.length === 0) {
    return (
      <StickToBottom key={resetKey} className="msg-scroll-frame" initial="smooth" resize="smooth">
        <StickToBottom.Content
          scrollClassName="panel-scroll"
          className="msg-stream msg-stream-empty"
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
      initial="smooth"
      resize="smooth"
    >
      <StickToBottom.Content scrollClassName="panel-scroll" className="msg-stream">
        <AnimatePresence initial={false}>
          {messages.map((m) => (
            // No `layout` prop — Motion's layout animation re-tweens
            // the block on every text delta, making the whole bubble
            // (avatar included) bobble while streaming. enterUp is
            // enough: first paint slides in, then the block grows
            // naturally with the DOM.
            <motion.div key={m.id} {...enterUp}>
              <MessageBlock msg={m} ctx={ctx} />
            </motion.div>
          ))}
        </AnimatePresence>
      </StickToBottom.Content>
      <ControlsRelay onChange={onControlsChange} />
    </StickToBottom>
  );
}

// Kept for any old callsite still importing the type. Most consumers
// should use `StreamControls` directly.
export type MessageStreamRef = Ref<HTMLDivElement>;
