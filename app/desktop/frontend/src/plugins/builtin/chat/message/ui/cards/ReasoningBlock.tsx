import type { BlockStatus } from "@/plugins/builtin/agent/public/viewState";
import { useEffect, useRef, useState } from "react";
import { MarkdownMessage } from "../markdown/MarkdownMessage";
import { Collapsible, Icon } from "@/ui";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";

interface Props {
  text: string;
  status: BlockStatus;
}

// Collapsible "thinking" panel. Auto-opens while the agent streams, then
// collapses once the reasoning is done. User can toggle anytime to override.
//
// Hand-rolled rather than Base UI Collapsible (CLAUDE.md §4 exemption c):
// the open state is derived — streaming drives it until the user's first
// toggle takes over — which the controlled/uncontrolled split can't
// express, and a disclosure header needs no focus/keyboard management
// beyond the native <button>.
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
  const [scrollTop, setScrollTop] = useState(0);
  const [scrollHeight, setScrollHeight] = useState(0);
  const [clientHeight, setClientHeight] = useState(0);

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
      // Eagerly update metrics so fade states stay in sync.
      setScrollTop(scrollEl.scrollTop);
      setScrollHeight(scrollEl.scrollHeight);
      setClientHeight(scrollEl.clientHeight);
    };
    pin();
    const ro = new ResizeObserver(pin);
    ro.observe(contentEl);
    return () => ro.disconnect();
  }, [streaming]);

  // Keep metrics in sync when content or open state changes.
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;
    setScrollTop(el.scrollTop);
    setScrollHeight(el.scrollHeight);
    setClientHeight(el.clientHeight);
  }, [text, isOpen]);

  const handleScroll = () => {
    const el = scrollRef.current;
    if (!el) return;
    setScrollTop(el.scrollTop);
    setScrollHeight(el.scrollHeight);
    setClientHeight(el.clientHeight);
  };

  const hasOverflow = scrollHeight > clientHeight;
  const atBottom = scrollHeight - scrollTop - clientHeight < 4;
  const showTopFade = isOpen && scrollTop > 0;
  const showBottomFade = isOpen && streaming && hasOverflow && !atBottom;

  return (
    <div data-slot="reasoning-root" className="my-1">
      <button
        type="button"
        onClick={toggle}
        data-slot="reasoning-trigger"
        className="inline-flex max-w-full items-center gap-2 rounded-md border-0 px-2 py-1 font-mono text-[12px] font-medium text-fg-faint transition-colors duration-150 hover:bg-fg/[0.02] hover:text-fg active:bg-fg/[0.04]"
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
      {/* Collapsible (grid-rows), not a height:auto tween — this block lives
          inside the message stream, where FM's auto-measure makes
          use-stick-to-bottom clamp the chat to the top (see Collapsible). */}
      <Collapsible open={isOpen}>
        <div
          data-slot="reasoning-content"
          ref={scrollRef}
          onScroll={handleScroll}
          className={cn(
            "relative overflow-hidden",
            streaming && isOpen && "max-h-48 overflow-y-auto",
          )}
        >
          {/* Top fade — visible when scrolled down */}
          <div
            data-slot="reasoning-fade-top"
            className={cn(
              "pointer-events-none absolute inset-x-0 top-0 z-10 h-6",
              "bg-[linear-gradient(to_bottom,var(--color-bg),transparent)]",
              "transition-opacity duration-[var(--dur-fast)]",
              showTopFade ? "opacity-100" : "opacity-0",
            )}
          />
          <div
            ref={contentRef}
            className="whitespace-pre-wrap px-0 pb-1 pt-1.5 text-[13px] italic leading-[1.6] text-fg-muted"
          >
            <MarkdownMessage text={text} streaming={streaming} />
            {status === "incomplete" && (
              <div className="mt-1 font-mono text-[11px] text-fg-faint">
                <Icon name="x" size={10} /> {t("reasoning.interrupted")}
              </div>
            )}
          </div>
          {/* Bottom fade — visible while streaming and not at bottom */}
          <div
            data-slot="reasoning-fade-bottom"
            className={cn(
              "pointer-events-none absolute inset-x-0 bottom-0 z-10 h-6",
              "bg-[linear-gradient(to_top,var(--color-bg),transparent)]",
              "transition-opacity duration-[var(--dur-fast)]",
              showBottomFade ? "opacity-100" : "opacity-0",
            )}
          />
        </div>
      </Collapsible>
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
  return `${s.slice(0, n).trimEnd()}…`;
}
