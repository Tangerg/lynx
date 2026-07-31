import type { BlockStatus } from "@/plugins/builtin/agent/public/viewState";
import { useCallback, useEffect, useRef, useState } from "react";
import { MarkdownMessage } from "../markdown/MarkdownMessage";
import { Button, Icon, StatusDot } from "@/ui";
import { AgentActivityDisclosure } from "@/ui/agent";
import { stopCurrentRootRun } from "@/plugins/builtin/agent/public/run";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/classNames";

interface Props {
  text: string;
  status: BlockStatus;
}

// Collapsible "thinking" panel. Auto-opens while the agent streams, then
// collapses once the reasoning is done. User can toggle anytime to override.
//
// AgentActivityDisclosure owns the Base UI disclosure/button semantics. This
// feature owns only the derived policy: streaming drives `open` until the
// user's first toggle takes over.
//
// Elapsed time is captured client-side: we snapshot the wall clock at first
// render (≈ first reasoning delta) and freeze it the tick streaming flips
// false. Server-authoritative duration would be cleaner, but reasoning
// timestamps aren't in the protocol events today and a 50ms render skew on a
// label that always reads "thought for Xs" is not worth a protocol change.
//
// Streaming auto-follow (ResizeObserver pin-to-bottom) + top/bottom gradient
// fades ported from assistant-ui canonical reasoning component technique.
export function ReasoningBlock({ text, status }: Props) {
  const t = useT();
  const streaming = status === "running";
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
      ? t("reasoning.thinkingWithTime", { time: elapsedLabel })
      : t("reasoning.thinking")
    : elapsedLabel
      ? t("reasoning.thoughtFor", { time: elapsedLabel })
      : t("reasoning.thought");
  const preview = streaming ? "" : truncate(text, 80);

  // ---- Bounded scroll + auto-follow + fades ----
  const scrollRef = useRef<HTMLDivElement>(null);
  const contentRef = useRef<HTMLDivElement>(null);
  // The three fades need three booleans, not three pixel counts. Measuring
  // positions put a state write on every scroll tick of a box that auto-follows a
  // stream — so it re-rendered the reasoning body continuously while tokens
  // arrived. Reduced at the measure site and compared before it is stored, a scroll
  // that doesn't cross a threshold re-renders nothing.
  const [edges, setEdges] = useState({ scrolled: false, atBottom: true, overflowing: false });
  const measure = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    const next = {
      scrolled: el.scrollTop > 0,
      atBottom: el.scrollHeight - el.scrollTop - el.clientHeight < 4,
      overflowing: el.scrollHeight > el.clientHeight,
    };
    setEdges((prev) =>
      prev.scrolled === next.scrolled &&
      prev.atBottom === next.atBottom &&
      prev.overflowing === next.overflowing
        ? prev
        : next,
    );
  }, []);

  // ResizeObserver: pin to bottom while streaming so new tokens stay visible.
  useEffect(() => {
    if (!streaming) return;
    const scrollEl = scrollRef.current;
    const contentEl = contentRef.current;
    if (!scrollEl || !contentEl) return;
    // Pin only when the user is already at the bottom — if they've scrolled
    // up to read, new tokens must not yank them back. Re-arms automatically:
    // the next content growth after they scroll back down pins again.
    const pin = () => {
      const distanceFromBottom = scrollEl.scrollHeight - scrollEl.scrollTop - scrollEl.clientHeight;
      if (distanceFromBottom < 4) {
        scrollEl.scrollTop = scrollEl.scrollHeight;
      }
      // Eagerly update the edges so fade states stay in sync.
      measure();
    };
    pin();
    const ro = new ResizeObserver(pin);
    ro.observe(contentEl);
    return () => ro.disconnect();
  }, [streaming, measure]);

  // Keep the edges in sync when content or open state changes.
  useEffect(() => {
    measure();
  }, [text, isOpen, measure]);

  const showTopFade = isOpen && edges.scrolled;
  const showBottomFade = isOpen && streaming && edges.overflowing && !edges.atBottom;

  return (
    <AgentActivityDisclosure
      icon="sparkle"
      label={<span className="[font-feature-settings:'tnum']">{label}</span>}
      detail={!isOpen && preview ? preview : undefined}
      trailing={streaming && isOpen ? <StatusDot tone="running" /> : undefined}
      actions={
        streaming ? (
          <Button
            variant="ghost"
            size="xs"
            onClick={() => {
              stopCurrentRootRun();
            }}
            className="text-fg-muted"
          >
            {t("reasoning.answerNow")}
          </Button>
        ) : undefined
      }
      open={isOpen}
      onToggle={toggle}
      className="my-1.5"
      contentClassName="relative"
    >
      <div
        ref={scrollRef}
        onScroll={measure}
        className={cn(
          "relative overflow-hidden pr-2",
          streaming && isOpen && "max-h-48 overflow-y-auto",
        )}
      >
        {/* Top fade — visible when scrolled down. */}
        <div
          className={cn(
            "pointer-events-none absolute inset-x-0 top-0 z-10 h-6",
            "bg-[linear-gradient(to_bottom,var(--color-canvas),transparent)]",
            "transition-opacity duration-[var(--dur-fast)]",
            showTopFade ? "opacity-100" : "opacity-0",
          )}
        />
        <div
          ref={contentRef}
          className="whitespace-pre-wrap text-ui-md leading-relaxed text-fg-muted"
        >
          <MarkdownMessage text={text} streaming={streaming} />
          {status === "incomplete" && (
            <div className="mt-1 font-mono text-ui-sm text-fg-faint">
              <Icon name="x" size={10} /> {t("reasoning.interrupted")}
            </div>
          )}
        </div>
        {/* Bottom fade — visible while streaming and not at bottom. */}
        <div
          className={cn(
            "pointer-events-none absolute inset-x-0 bottom-0 z-10 h-6",
            "bg-[linear-gradient(to_top,var(--color-canvas),transparent)]",
            "transition-opacity duration-[var(--dur-fast)]",
            showBottomFade ? "opacity-100" : "opacity-0",
          )}
        />
      </div>
    </AgentActivityDisclosure>
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
  return `${s.slice(0, n).trimEnd()}…`;
}
