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
  /** The turn has started answering. Thinking is then the account of how the answer
   *  was reached, not the thing to read, so it folds away without waiting for its own
   *  terminal status — some models keep a reasoning block open while prose streams. */
  superseded?: boolean;
}

/** Several times the most characters the collapsed row can ever show — see the
 *  `preview` comment. Raising or lowering it changes nothing on screen. */
const PREVIEW_LAYOUT_BOUND = 400;

/** The thought as it stands right now — the last line with anything on it, so a
 *  trailing newline mid-stream does not blank the row it is reporting into. */
function lastLine(text: string): string {
  const lines = text.split("\n");
  for (let index = lines.length - 1; index >= 0; index -= 1) {
    const line = lines[index]!.trim();
    if (line !== "") return line;
  }
  return "";
}

// Collapsible "thinking" aside. Auto-opens while the agent streams, then collapses
// once the reasoning is done. User can toggle anytime to override.
//
// The `line` shell, and a left rule on the body. Reasoning is an ASIDE, not an
// activity: it produced nothing, it acted on nothing, and it is the one block in a
// turn that is prose rather than data. Wearing the same card as a tool call was the
// single loudest reason a transcript read as one grey stack — both references say
// the same thing about it and neither gives it a card: quiet ink, an indent, a rule
// down the side, and the roomiest leading in the ladder.
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
export function ReasoningBlock({ text, status, superseded = false }: Props) {
  const t = useT();
  const streaming = status === "running";
  const [open, setOpen] = useState(true);
  const [userToggled, setUserToggled] = useState(false);
  const isOpen = userToggled ? open : streaming && !superseded;

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
  // The word and the number are separate slots now: the word is a label and the
  // number is data, so the number can sit in the row's mono meta column with
  // every other duration instead of being sentence-cased into the label.
  const label = streaming ? t("reasoning.thinking") : t("reasoning.thought");
  // Where the preview ends is the ROW's business, and the row already answers it
  // in CSS at the real edge of the real column. A character count here was a
  // second answer to the same question, and a worse one: it stopped short of that
  // edge, so a reasoning row trailed off mid-row while the tool row beside it —
  // same component, CSS truncation — ran the full width.
  //
  // The slice that remains is a cost bound, not a display policy. It cannot be
  // observed: the widest this row ever gets is the reading column, which holds
  // ~110 Latin characters at this size, and a settled 50k-token thought would
  // otherwise lay out as a 50k-character nowrap line once per collapsed row.
  //
  // While it is still thinking the preview is the TAIL, not the head: a folded row
  // showing the opening of a thought that has run for twenty seconds is a row that
  // stopped reporting. It used to show nothing at all in that state, which is what a
  // folded row looked like for the whole time the model was working.
  const preview = streaming
    ? lastLine(text).slice(-PREVIEW_LAYOUT_BOUND)
    : text.slice(0, PREVIEW_LAYOUT_BOUND);

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
      shell="line"
      label={label}
      detail={!isOpen && preview ? preview : undefined}
      trailing={
        <>
          {elapsedLabel}
          {streaming && isOpen ? <StatusDot tone="running" /> : null}
        </>
      }
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
      // The rule replaces the card: it marks the aside's extent down the margin
      // instead of boxing it, so a long chain of thought does not become the
      // largest object in the turn. `border-field` and not `border-divider` — a
      // divider separates peers in a list, and at 7% ink it does not read as a
      // margin rule; this is the same job `.md blockquote` does, one step up.
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
            "pointer-events-none absolute inset-x-0 top-0 z-1 h-6",
            "bg-[linear-gradient(to_bottom,var(--app-content-surface),transparent)]",
            "transition-opacity duration-[var(--dur-fast)]",
            showTopFade ? "opacity-100" : "opacity-0",
          )}
        />
        <div
          ref={contentRef}
          className="whitespace-pre-wrap text-ui-sm leading-prose text-fg-muted"
        >
          <MarkdownMessage text={text} streaming={streaming} />
          {status === "incomplete" && (
            <div className="mt-1 font-mono text-ui-sm text-fg-faint">
              <Icon name="x" size="xs" /> {t("reasoning.interrupted")}
            </div>
          )}
        </div>
        {/* Bottom fade — visible while streaming and not at bottom. */}
        <div
          className={cn(
            "pointer-events-none absolute inset-x-0 bottom-0 z-1 h-6",
            "bg-[linear-gradient(to_top,var(--app-content-surface),transparent)]",
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
