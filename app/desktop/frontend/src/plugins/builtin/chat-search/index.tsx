// Built-in plugin: Cmd+F in-chat search.
//
// Opens a slim overlay anchored to the top-right of the chat panel.
// Walks every `.msg-content` text node in the document, finds matches,
// and paints them via the CSS Custom Highlight API (WKWebView /
// Chromium / WebKit2GTK all support it as of mid-2024). The active
// match scrolls into view; Enter / Shift+Enter cycle through hits.

import { useEffect, useRef, useState } from "react";
import { Icon, Tooltip } from "@/components/common";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";
import { SHORTCUT } from "@/plugins/sdk/kernelPoints";

const OPEN_EVENT = "lyra.chat-search.open";

function escapeRegExp(s: string): string {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

// Walk every `.msg-content` text node, find regex matches, return them
// as DOM Ranges. Ranges can be applied to CSS.highlights or scrolled into
// view via the surrounding element's getBoundingClientRect().
function findRanges(query: string): Range[] {
  if (!query) return [];
  const re = new RegExp(escapeRegExp(query), "gi");
  const out: Range[] = [];
  const roots = document.querySelectorAll<HTMLElement>(".msg-content");
  for (const root of roots) {
    const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
    let node = walker.nextNode();
    while (node) {
      const textNode = node;
      const text = textNode.nodeValue ?? "";
      for (const m of text.matchAll(re)) {
        const range = document.createRange();
        range.setStart(textNode, m.index);
        range.setEnd(textNode, m.index + m[0].length);
        out.push(range);
      }
      node = walker.nextNode();
    }
  }
  return out;
}

// CSS Highlight API gracefully degrades on environments that don't have
// it (WebKit2GTK older than 2.46) — we just skip painting and rely on
// scrollIntoView so search still does *something*.
const HIGHLIGHTS_AVAILABLE = typeof CSS !== "undefined" && "highlights" in CSS;

function paintHighlights(all: Range[], activeIdx: number): void {
  if (!HIGHLIGHTS_AVAILABLE) return;
  CSS.highlights.delete("chat-search");
  CSS.highlights.delete("chat-search-active");
  if (all.length === 0) return;
  const inactive = all.filter((_, i) => i !== activeIdx);
  if (inactive.length > 0) {
    CSS.highlights.set("chat-search", new Highlight(...inactive));
  }
  if (all[activeIdx]) {
    CSS.highlights.set("chat-search-active", new Highlight(all[activeIdx]));
  }
}

function clearHighlights(): void {
  if (!HIGHLIGHTS_AVAILABLE) return;
  CSS.highlights.delete("chat-search");
  CSS.highlights.delete("chat-search-active");
}

function ChatSearchOverlay() {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [matches, setMatches] = useState<Range[]>([]);
  const [active, setActive] = useState(0);
  const inputRef = useRef<HTMLInputElement>(null);

  // Listen for the shortcut. Stable handler; mount/unmount once.
  useEffect(() => {
    const open = () => setOpen(true);
    window.addEventListener(OPEN_EVENT, open);
    return () => window.removeEventListener(OPEN_EVENT, open);
  }, []);

  // Focus input when the overlay opens.
  useEffect(() => {
    if (open) {
      inputRef.current?.focus();
      inputRef.current?.select();
    } else {
      // Clean up paint when closing.
      clearHighlights();
      setQuery("");
      setMatches([]);
      setActive(0);
    }
  }, [open]);

  // Re-search whenever the query changes. Reset active to 0 so the user
  // sees the first hit.
  useEffect(() => {
    if (!open) return;
    const found = findRanges(query);
    setMatches(found);
    setActive(0);
    paintHighlights(found, 0);
    if (found[0]) {
      found[0].startContainer.parentElement?.scrollIntoView({
        block: "center",
        behavior: "smooth",
      });
    }
  }, [query, open]);

  // Re-paint when navigating between hits.
  useEffect(() => {
    paintHighlights(matches, active);
    if (matches[active]) {
      matches[active].startContainer.parentElement?.scrollIntoView({
        block: "center",
        behavior: "smooth",
      });
    }
  }, [active, matches]);

  // Cleanup highlights on unmount.
  useEffect(() => clearHighlights, []);

  if (!open) return null;

  const total = matches.length;
  const next = () => total > 0 && setActive((i) => (i + 1) % total);
  const prev = () => total > 0 && setActive((i) => (i - 1 + total) % total);

  return (
    <search
      className={cn(
        "fixed top-3 right-4 z-50 inline-flex items-center gap-1 rounded-lg border border-line bg-surface px-2 py-1.5 shadow-lg",
        "[-webkit-app-region:no-drag] [--wails-draggable:no-drag]",
      )}
    >
      <input
        ref={inputRef}
        type="text"
        aria-label="Search in chat"
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        placeholder="Search in chat…"
        className="h-7 w-56 rounded-md border-0 bg-transparent px-2 font-sans text-[13px] text-fg outline-none placeholder:text-fg-faint"
        onKeyDown={(e) => {
          if (e.key === "Escape") {
            e.preventDefault();
            setOpen(false);
          } else if (e.key === "Enter") {
            e.preventDefault();
            if (e.shiftKey) prev();
            else next();
          }
        }}
      />
      <span className="px-1.5 font-mono text-[11px] tabular-nums text-fg-faint">
        {total > 0 ? `${active + 1} / ${total}` : query ? "0 / 0" : ""}
      </span>
      <Tooltip label="Previous match (⇧⏎)">
        <button
          type="button"
          onClick={prev}
          disabled={total === 0}
          aria-label="Previous match"
          className="grid h-6 w-6 place-items-center rounded border-0 bg-transparent text-fg-muted cursor-pointer transition-colors hover:bg-surface-2 hover:text-fg disabled:cursor-not-allowed disabled:opacity-40"
        >
          <Icon name="chevron-up" size={12} />
        </button>
      </Tooltip>
      <Tooltip label="Next match (⏎)">
        <button
          type="button"
          onClick={next}
          disabled={total === 0}
          aria-label="Next match"
          className="grid h-6 w-6 place-items-center rounded border-0 bg-transparent text-fg-muted cursor-pointer transition-colors hover:bg-surface-2 hover:text-fg disabled:cursor-not-allowed disabled:opacity-40"
        >
          <Icon name="chevron-down" size={12} />
        </button>
      </Tooltip>
      <Tooltip label="Close (Esc)">
        <button
          type="button"
          onClick={() => setOpen(false)}
          aria-label="Close"
          className="grid h-6 w-6 place-items-center rounded border-0 bg-transparent text-fg-muted cursor-pointer transition-colors hover:bg-surface-2 hover:text-fg"
        >
          <Icon name="x" size={12} />
        </button>
      </Tooltip>
    </search>
  );
}

export default definePlugin({
  name: "lyra.builtin.chat-search",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("app.overlay", {
      id: "chat-search",
      order: 50,
      component: ChatSearchOverlay,
    });
    host.extensions.contribute(SHORTCUT, {
      key: "Mod+F",
      description: "Find in chat",
      // The shortcut must work even when the composer textarea is
      // focused — that's where users naturally are when reading chat.
      allowInInputs: true,
      handler: (e) => {
        e.preventDefault();
        window.dispatchEvent(new Event(OPEN_EVENT));
      },
    });
  },
});
