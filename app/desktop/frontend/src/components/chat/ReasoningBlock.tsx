import { useEffect, useRef, useState } from "react";
import { AnimatePresence, motion } from "motion/react";
import { Icon } from "@/components/common";
import { MarkdownMessage } from "@/components/chat/MarkdownMessage";
import { swift } from "@/lib/motion";

type Props = {
  text: string;
  streaming: boolean;
};

// Collapsible "thinking" panel. Auto-opens while the agent streams, then
// collapses once the reasoning is done. User can toggle anytime to override.
//
// Elapsed time is captured client-side: we snapshot the wall clock at first
// render (≈ first reasoning delta) and freeze it the tick streaming flips
// false. Server-authoritative duration would be cleaner, but reasoning
// timestamps aren't in the AG-UI events today and a 50ms render skew on a
// label that always reads "thought for Xs" is not worth a protocol change.
export function ReasoningBlock({ text, streaming }: Props) {
  const [open, setOpen] = useState(true);
  const [userToggled, setUserToggled] = useState(false);
  const isOpen = userToggled ? open : streaming;

  const toggle = () => {
    setUserToggled(true);
    setOpen((v) => !v);
  };

  const startedAtRef = useRef<number>(Date.now());
  const [elapsedMs, setElapsedMs] = useState<number | null>(null);

  // While streaming, tick once a second so the header counter advances.
  // When streaming ends, freeze the value — that's the final "thought for X".
  useEffect(() => {
    if (!streaming) {
      setElapsedMs(Date.now() - startedAtRef.current);
      return;
    }
    const tick = () => setElapsedMs(Date.now() - startedAtRef.current);
    tick();
    const id = window.setInterval(tick, 1000);
    return () => window.clearInterval(id);
  }, [streaming]);

  const elapsedLabel = formatElapsed(elapsedMs);
  const label = streaming
    ? (elapsedLabel ? `Thinking · ${elapsedLabel}` : "Thinking…")
    : (elapsedLabel ? `Thought for ${elapsedLabel}` : "Thought");
  const preview = streaming ? "" : truncate(text, 80);

  return (
    <div className={`reasoning-block ${isOpen ? "open" : "closed"}`}>
      <button className="reasoning-head" onClick={toggle} type="button">
        <Icon name="sparkle" size={11} />
        <span className="rb-label">{label}</span>
        {!isOpen && preview && <span className="rb-preview">{preview}</span>}
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
              <MarkdownMessage text={text} streaming={streaming} />
            </div>
          </motion.div>
        )}
      </AnimatePresence>
    </div>
  );
}

function formatElapsed(ms: number | null): string | null {
  if (ms == null || ms < 500) return null;
  const sec = Math.round(ms / 1000);
  if (sec < 60) return `${sec}s`;
  const m = Math.floor(sec / 60);
  const s = sec % 60;
  return s === 0 ? `${m}m` : `${m}m${s}s`;
}

function truncate(s: string, n: number): string {
  if (s.length <= n) return s;
  return s.slice(0, n).trimEnd() + "…";
}
