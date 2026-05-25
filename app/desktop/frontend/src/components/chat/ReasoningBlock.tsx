import { AnimatePresence, motion } from "motion/react";
import { useEffect, useRef, useState } from "react";
import { MarkdownMessage } from "@/components/chat/MarkdownMessage";
import { Icon } from "@/components/common";
import { swift } from "@/lib/motion";

interface Props {
  text: string;
  streaming: boolean;
}

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

  // Flip relative to what the user *sees* (isOpen), not the underlying
  // `open` slot. Before first toggle, `isOpen` follows `streaming` while
  // `open` is still the initial `true` — flipping `open` would land on
  // the same state the user already sees and the first click would feel
  // dead. Anchoring on isOpen makes every click match its arrow.
  const toggle = () => {
    setUserToggled(true);
    setOpen(!isOpen);
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
    ? elapsedLabel
      ? `Thinking · ${elapsedLabel}`
      : "Thinking…"
    : elapsedLabel
      ? `Thought for ${elapsedLabel}`
      : "Thought";
  const preview = streaming ? "" : truncate(text, 80);

  return (
    <div className="my-2 rounded-sm bg-surface-2 px-3 py-2">
      <button
        type="button"
        onClick={toggle}
        className="inline-flex max-w-full items-center gap-2 rounded-md bg-transparent border-0 px-2.5 py-1.5 font-mono text-[12px] font-semibold text-fg-faint cursor-pointer transition-colors duration-150 hover:bg-[color-mix(in_srgb,var(--color-text)_6%,transparent)] hover:text-fg active:bg-[color-mix(in_srgb,var(--color-text)_10%,transparent)]"
      >
        <Icon name="sparkle" size={11} />
        <span className="shrink-0 [font-feature-settings:'tnum']">{label}</span>
        {!isOpen && preview && (
          <span className="min-w-0 overflow-hidden text-ellipsis whitespace-nowrap font-mono text-[11.5px] font-normal text-fg-faint">
            {preview}
          </span>
        )}
        {streaming && isOpen && (
          <span className="h-1.5 w-1.5 rounded-full bg-accent shadow-[0_0_6px_var(--color-accent)] animate-pulse-dot" />
        )}
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
            <div className="whitespace-pre-wrap px-0 pb-1 pt-1.5 text-[14px] italic leading-[1.6] text-fg-muted">
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
  return `${s.slice(0, n).trimEnd()  }…`;
}
