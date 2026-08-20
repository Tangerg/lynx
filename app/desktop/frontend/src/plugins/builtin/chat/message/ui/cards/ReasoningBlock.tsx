import type { BlockStatus } from "@/plugins/builtin/agent/public/viewState";
import { useCallback, useEffect, useRef, useState } from "react";
import { MarkdownMessage } from "../markdown/MarkdownMessage";
import { Icon, Loader } from "@/ui";
import { AgentActivityDisclosure } from "@/ui/agent";
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
// Streaming auto-follow (ResizeObserver pin-to-bottom) + top/bottom gradient
// fades ported from assistant-ui canonical reasoning component technique.
export function ReasoningBlock({ text, status, superseded = false }: Props) {
  const t = useT();
  const streaming = status === "running";
  // null delegates to the domain policy; a boolean is the user's explicit
  // override. This is one state machine, not two booleans that can disagree.
  const [openOverride, setOpenOverride] = useState<boolean | null>(null);
  const isOpen = openOverride ?? (streaming && !superseded);

  // Flip relative to what the user *sees* (isOpen), not the underlying
  // automatic policy. Anchoring on isOpen makes every click match its arrow.
  const toggle = () => {
    setOpenOverride(!isOpen);
  };

  const label = streaming ? t("reasoning.thinking") : t("reasoning.thought");

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
      label={streaming ? <Loader variant="text-shimmer" size="sm" text={label} /> : label}
      toggleLabel={label}
      open={isOpen}
      onToggle={toggle}
      // The rule replaces the card: it marks the aside's extent down the margin
      // instead of boxing it, so a long chain of thought does not become the
      // largest object in the turn. `border-field` and not `border-divider` — a
      // divider separates peers in a list, and at 7% ink it does not read as a
      // margin rule; this is the same job `.md blockquote` does, one step up.
      contentClassName="ml-5 border-l border-field pt-0.5 pl-6"
    >
      <div
        ref={scrollRef}
        // Overflow regions need a keyboard entry point; the linter cannot infer
        // conditional scrollability from the utility classes below.
        // oxlint-disable-next-line jsx-a11y/no-noninteractive-tabindex
        tabIndex={streaming && isOpen ? 0 : undefined}
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
          <MarkdownMessage text={text} streaming={streaming} reveal="smooth" />
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
