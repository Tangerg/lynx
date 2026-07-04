import { useEffect, useRef, useState } from "react";
import { Icon, Tooltip } from "@/ui";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import { useActiveSessionId } from "@/plugins/builtin/agent/public/session";
import { clearChatSearchHighlights, paintChatSearchHighlights } from "../application/highlights";
import { CHAT_SEARCH_OPEN_EVENT } from "../application/openChatSearch";
import { findMessageRanges } from "../application/ranges";

export function ChatSearchOverlay() {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [matches, setMatches] = useState<Range[]>([]);
  const [active, setActive] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    const onOpen = () => setOpen(true);
    window.addEventListener(CHAT_SEARCH_OPEN_EVENT, onOpen);
    return () => window.removeEventListener(CHAT_SEARCH_OPEN_EVENT, onOpen);
  }, []);

  const activeSessionId = useActiveSessionId();
  useEffect(() => {
    // Ranges point into the previous session's message DOM after a switch.
    setOpen(false);
  }, [activeSessionId]);

  useEffect(() => {
    if (open) {
      inputRef.current?.focus();
      inputRef.current?.select();
    } else {
      clearChatSearchHighlights();
      setQuery("");
      setMatches([]);
      setActive(0);
    }
  }, [open]);

  useEffect(() => {
    if (!open) return;

    const found = findMessageRanges(query);
    setMatches(found);
    setActive(0);
    paintChatSearchHighlights(found, 0);
    scrollRangeIntoView(found[0]);
  }, [query, open]);

  useEffect(() => {
    paintChatSearchHighlights(matches, active);
    scrollRangeIntoView(matches[active]);
  }, [active, matches]);

  useEffect(() => clearChatSearchHighlights, []);

  const t = useT();
  if (!open) return null;

  const total = matches.length;
  const next = () => total > 0 && setActive((index) => (index + 1) % total);
  const prev = () => total > 0 && setActive((index) => (index - 1 + total) % total);

  return (
    <search
      className={cn(
        "fixed top-3 right-4 z-50 inline-flex items-center gap-1 rounded-lg bg-surface px-2 py-1.5 shadow-[var(--shadow-popover)]",
        "[-webkit-app-region:no-drag] [--wails-draggable:no-drag]",
      )}
    >
      <input
        ref={inputRef}
        type="text"
        aria-label={t("chatSearch.label")}
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        placeholder={t("chatSearch.placeholder")}
        className="h-7 w-56 rounded-md border-0 bg-transparent px-2 font-sans text-[13px] text-fg outline-none placeholder:text-fg-faint"
        onKeyDown={(event) => {
          if (event.nativeEvent.isComposing) return;
          if (event.key === "Escape") {
            event.preventDefault();
            setOpen(false);
          } else if (event.key === "Enter") {
            event.preventDefault();
            if (event.shiftKey) prev();
            else next();
          }
        }}
      />
      <span className="px-1.5 font-mono text-[11px] text-fg-faint">
        {total > 0 ? `${active + 1} / ${total}` : query ? "0 / 0" : ""}
      </span>
      <Tooltip label={`${t("chatSearch.prev")} (⇧⏎)`}>
        <button
          type="button"
          onClick={prev}
          disabled={total === 0}
          aria-label={t("chatSearch.prev")}
          className="grid h-6 w-6 place-items-center rounded border-0 bg-transparent text-fg-muted transition-colors hover:bg-surface-2 hover:text-fg disabled:cursor-not-allowed disabled:opacity-40"
        >
          <Icon name="chevron-up" size={12} />
        </button>
      </Tooltip>
      <Tooltip label={`${t("chatSearch.next")} (⏎)`}>
        <button
          type="button"
          onClick={next}
          disabled={total === 0}
          aria-label={t("chatSearch.next")}
          className="grid h-6 w-6 place-items-center rounded border-0 bg-transparent text-fg-muted transition-colors hover:bg-surface-2 hover:text-fg disabled:cursor-not-allowed disabled:opacity-40"
        >
          <Icon name="chevron-down" size={12} />
        </button>
      </Tooltip>
      <Tooltip label={`${t("common.close")} (Esc)`}>
        <button
          type="button"
          onClick={() => setOpen(false)}
          aria-label={t("common.close")}
          className="grid h-6 w-6 place-items-center rounded border-0 bg-transparent text-fg-muted transition-colors hover:bg-surface-2 hover:text-fg"
        >
          <Icon name="x" size={12} />
        </button>
      </Tooltip>
    </search>
  );
}

function scrollRangeIntoView(range: Range | undefined): void {
  range?.startContainer.parentElement?.scrollIntoView({
    block: "center",
    behavior: "smooth",
  });
}
