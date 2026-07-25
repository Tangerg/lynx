import { useEffect, useRef, useState } from "react";
import { IconButton, TextField } from "@/ui";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";
import { useActiveSessionId } from "@/plugins/builtin/agent/public/session";
import { clearChatSearchHighlights, paintChatSearchHighlights } from "../adapters/searchHighlights";
import { setChatSearchOpener } from "../application/openChatSearch";
import { findMessageRanges } from "../adapters/messageRanges";

export function ChatSearchOverlay() {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [matches, setMatches] = useState<Range[]>([]);
  const [active, setActive] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    setChatSearchOpener(() => setOpen(true));
    return () => setChatSearchOpener(null);
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
      <TextField
        ref={inputRef}
        variant="bare"
        font="sans"
        size="lg"
        aria-label={t("chatSearch.label")}
        value={query}
        onChange={(event) => setQuery(event.target.value)}
        placeholder={t("chatSearch.placeholder")}
        className="h-7 w-56 px-2"
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
      <span className="px-1.5 font-mono text-ui-sm text-fg-faint">
        {total > 0 ? `${active + 1} / ${total}` : query ? "0 / 0" : ""}
      </span>
      <IconButton
        icon="chevron-up"
        size="xs"
        title={`${t("chatSearch.prev")} (⇧⏎)`}
        aria-label={t("chatSearch.prev")}
        disabled={total === 0}
        onClick={prev}
      />
      <IconButton
        icon="chevron-down"
        size="xs"
        title={`${t("chatSearch.next")} (⏎)`}
        aria-label={t("chatSearch.next")}
        disabled={total === 0}
        onClick={next}
      />
      <IconButton
        icon="x"
        size="xs"
        title={`${t("common.close")} (Esc)`}
        aria-label={t("common.close")}
        onClick={() => setOpen(false)}
      />
    </search>
  );
}

function scrollRangeIntoView(range: Range | undefined): void {
  range?.startContainer.parentElement?.scrollIntoView({
    block: "center",
    behavior: "smooth",
  });
}
