import { useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import { Icon } from "@/components/common";
import { FadeInText } from "@/components/chat/FadeInText";
import { swift } from "@/lib/motion";

type Props = {
  text: string;
  streaming: boolean;
};

// Collapsible "thinking" panel. Auto-opens while the agent streams, then
// collapses once the reasoning is done. User can toggle anytime to override.
export function ReasoningBlock({ text, streaming }: Props) {
  const [open, setOpen] = useState(true);
  const [userToggled, setUserToggled] = useState(false);
  const isOpen = userToggled ? open : streaming;

  const toggle = () => {
    setUserToggled(true);
    setOpen((v) => !v);
  };

  const preview = streaming ? "Thinking…" : truncate(text, 80);

  return (
    <div className={`reasoning-block ${isOpen ? "open" : "closed"}`}>
      <button className="reasoning-head" onClick={toggle} type="button">
        <Icon name="sparkle" size={11} />
        <span className="rb-label">Reasoning</span>
        {!isOpen && <span className="rb-preview">{preview}</span>}
        {streaming && isOpen && <span className="rb-pulse" />}
      </button>
      <AnimatePresence initial={false}>
        {isOpen && (
          <motion.div
            key="body"
            initial={{ height: 0, opacity: 0 }}
            animate={{ height: "auto", opacity: 1 }}
            exit={{ height: 0, opacity: 0 }}
            transition={swift}
            style={{ overflow: "hidden" }}
          >
            <div className="reasoning-body">
              <FadeInText text={text} streaming={streaming} />
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}

function truncate(s: string, n: number): string {
  if (s.length <= n) return s;
  return s.slice(0, n).trimEnd() + "…";
}
