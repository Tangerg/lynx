import type {Ref} from "react";
import type { PartCtx } from "./PartRenderer";
import type { Message } from "@/protocol/agui/viewState";
import { AnimatePresence, motion } from "motion/react";
import {  useCallback, useEffect } from "react";
import { StickToBottom, useStickToBottomContext } from "use-stick-to-bottom";
import { enterUp } from "@/lib/motion";
import { Slot } from "@/plugins/Slot";
import { MessageBlock } from "./MessageBlock";

// Chat scroll surface, backed by use-stick-to-bottom. `resetKey`
// re-keys the subtree on session switch so a new thread lands at the
// bottom. Follow state surfaces to the parent via ControlsRelay below.

export interface StreamControls {
  isAtBottom: boolean;
  scrollToBottom: () => void;
}

interface Props {
  messages: Message[];
  ctx: PartCtx;
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
