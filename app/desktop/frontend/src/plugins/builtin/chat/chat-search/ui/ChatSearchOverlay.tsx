import { useEffect, useRef, useState } from "react";
import { IconButton, TextField } from "@/ui";
import { cn } from "@/lib/classNames";
import { useT } from "@/lib/i18n";
import { useActiveSessionId } from "@/plugins/builtin/agent/public/session";
import {
  clearChatSearchHighlights,
  installChatSearchHighlightStyles,
  paintChatSearchHighlights,
} from "../adapters/searchHighlights";
import { setChatSearchOpener } from "../application/openChatSearch";
import { findMessageRanges } from "../adapters/messageRanges";

export function ChatSearchOverlay() {
  const activeSessionId = useActiveSessionId();

  // A Range belongs to the DOM tree that created it. Remounting at the session
  // boundary makes that lifetime structural: no search state can survive into
  // a different transcript, even when both sessions render similar content.
  return <SessionChatSearchOverlay key={activeSessionId || "no-session"} />;
}

type SearchState = {
  query: string;
  matches: Range[];
  activeIndex: number;
};

const EMPTY_SEARCH: SearchState = {
  query: "",
  matches: [],
  activeIndex: 0,
};

function SessionChatSearchOverlay() {
  const [open, setOpen] = useState(false);
  const [search, setSearch] = useState<SearchState>(EMPTY_SEARCH);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    setChatSearchOpener(() => setOpen(true));
    return () => setChatSearchOpener(null);
  }, []);

  useEffect(() => {
    const uninstallStyles = installChatSearchHighlightStyles();
    return () => {
      clearChatSearchHighlights();
      uninstallStyles();
    };
  }, []);

  useEffect(() => {
    if (!open) return;
    inputRef.current?.focus();
    inputRef.current?.select();
  }, [open]);

  const close = () => {
    clearChatSearchHighlights();
    setSearch(EMPTY_SEARCH);
    setOpen(false);
  };

  const changeQuery = (query: string) => {
    const found = findMessageRanges(query);
    setSearch({ query, matches: found, activeIndex: 0 });
    paintChatSearchHighlights(found, 0);
    scrollRangeIntoView(found[0]);
  };

  const move = (delta: number) => {
    const { matches, activeIndex } = search;
    if (matches.length === 0) return;
    const nextIndex = (activeIndex + delta + matches.length) % matches.length;
    setSearch({ ...search, activeIndex: nextIndex });
    paintChatSearchHighlights(matches, nextIndex);
    scrollRangeIntoView(matches[nextIndex]);
  };

  const t = useT();
  if (!open) return null;

  const { query, matches, activeIndex } = search;
  const total = matches.length;

  return (
    <div
      role="search"
      className={cn(
        "fixed top-3 right-4 z-[var(--layer-floating)] inline-flex items-center gap-1 rounded-lg bg-card px-2 py-1.5 shadow-[var(--shadow-overlay)]",
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
        onChange={(event) => changeQuery(event.target.value)}
        placeholder={t("chatSearch.placeholder")}
        className="h-7 w-56 px-2"
        onKeyDown={(event) => {
          if (event.nativeEvent.isComposing) return;
          if (event.key === "Escape") {
            event.preventDefault();
            close();
          } else if (event.key === "Enter") {
            event.preventDefault();
            move(event.shiftKey ? -1 : 1);
          }
        }}
      />
      <span className="px-1.5 font-mono text-ui-sm text-fg-faint">
        {total > 0 ? `${activeIndex + 1} / ${total}` : query ? "0 / 0" : ""}
      </span>
      <IconButton
        icon="chevron-up"
        size="xs"
        title={`${t("chatSearch.prev")} (⇧⏎)`}
        aria-label={t("chatSearch.prev")}
        disabled={total === 0}
        onClick={() => move(-1)}
      />
      <IconButton
        icon="chevron-down"
        size="xs"
        title={`${t("chatSearch.next")} (⏎)`}
        aria-label={t("chatSearch.next")}
        disabled={total === 0}
        onClick={() => move(1)}
      />
      <IconButton
        icon="x"
        size="xs"
        title={`${t("common.close")} (Esc)`}
        aria-label={t("common.close")}
        onClick={close}
      />
    </div>
  );
}

function scrollRangeIntoView(range: Range | undefined): void {
  range?.startContainer.parentElement?.scrollIntoView({
    block: "center",
    behavior: "smooth",
  });
}
