import { forwardRef } from "react";
import { AnimatePresence, motion } from "motion/react";
import { ScrollArea } from "@/components/common";
import { enterUp } from "@/lib/motion";
import { Slot } from "@/plugins/Slot";
import type { Message } from "@/protocol/agui/viewState";
import { MessageBlock } from "./MessageBlock";
import type { PartCtx } from "./PartRenderer";

type Props = {
  messages: Message[];
  ctx: PartCtx;
};

export const MessageStream = forwardRef<HTMLDivElement, Props>(function MessageStream(
  { messages, ctx },
  ref,
) {
  // Empty conversation → render whatever plugins have contributed to the
  // welcome slot. Built-in `lyra.builtin.welcome-screen` provides a default;
  // a user plugin can replace or supplement it.
  if (messages.length === 0) {
    return (
      <ScrollArea ref={ref}>
        <div className="msg-stream msg-stream-empty">
          <Slot name="chat.empty" />
        </div>
      </ScrollArea>
    );
  }

  return (
    <ScrollArea ref={ref}>
      <div className="msg-stream">
        <AnimatePresence initial={false}>
          {messages.map((m) => (
            <motion.div key={m.id} {...enterUp} layout>
              <MessageBlock msg={m} ctx={ctx} />
            </motion.div>
          ))}
        </AnimatePresence>
      </div>
    </ScrollArea>
  );
});
