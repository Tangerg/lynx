import type { BlockCtx } from "../message";
import type { Message } from "@/protocol/run/viewState";
import { AnimatePresence, motion } from "motion/react";
import { useCallback, useEffect } from "react";
import { StickToBottom, useStickToBottomContext } from "use-stick-to-bottom";
import { enterUp } from "@/lib/motion";
import { Slot } from "@/plugins/host/Slot";
import { useAgentRunning } from "@/state/agentStore";
import { MessageBlock } from "../message";

// Chat scroll surface, backed by use-stick-to-bottom. `resetKey`
// re-keys the subtree on session switch so a new thread lands at the
// bottom. Follow state surfaces to the parent via ControlsRelay below.

export interface StreamControls {
  isAtBottom: boolean;
  scrollToBottom: () => void;
}

interface Props {
  messages: Message[];
  ctx: BlockCtx;
  /** Live model name for assistant turns (resolved in ChatStream). */
  assistantName?: string;
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

export function MessageStream({ messages, ctx, assistantName, resetKey, onControlsChange }: Props) {
  // While a run streams, content grows continuously; the default `resize`
  // spring (stiffness 0.05 / mass 1.25) is too sluggish to track it and the
  // tail scrolls out of view (D2). Hard-pin to the bottom during generation,
  // and keep the smooth catch-up only when idle (re-open / history load).
  // `running` flips only at run boundaries, so this never churns per token.
  const running = useAgentRunning();
  if (messages.length === 0) {
    return (
      <StickToBottom key={resetKey} className="msg-scroll-frame" initial="smooth" resize="smooth">
        <StickToBottom.Content
          scrollClassName="panel-scroll"
          className="relative mx-auto flex w-full max-w-[760px] flex-col gap-7 px-6 pt-6 pb-[220px]"
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
      resize={running ? "instant" : "smooth"}
    >
      <StickToBottom.Content
        scrollClassName="panel-scroll"
        className="relative mx-auto flex w-full max-w-[760px] flex-col gap-7 px-6 pt-6 pb-[220px]"
      >
        <AnimatePresence initial={false}>
          {messages.map((m) => (
            // No `layout` prop — Motion's layout animation re-tweens
            // the block on every text delta, making the whole bubble
            // (avatar included) bobble while streaming. enterUp is
            // enough: first paint slides in, then the block grows
            // naturally with the DOM.
            <motion.div key={m.id} {...enterUp}>
              <MessageBlock msg={m} ctx={ctx} assistantName={assistantName} />
            </motion.div>
          ))}
        </AnimatePresence>
      </StickToBottom.Content>
      <ControlsRelay onChange={onControlsChange} />
    </StickToBottom>
  );
}
