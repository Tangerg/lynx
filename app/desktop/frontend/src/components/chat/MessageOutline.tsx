// Floating heading outline for assistant messages. Anchored to the
// gutter outside the 760px content column so it never compresses
// content width. Hides itself below MIN_ITEMS (short messages don't
// need a TOC) and on narrow viewports where there is no gutter.

import type { RefObject } from "react";
import { useEffect, useState } from "react";
import { cn } from "@/lib/utils";

interface OutlineItem {
  id: string;
  level: number;
  text: string;
}

const MIN_ITEMS = 3;

export function MessageOutline({ target }: { target: RefObject<HTMLElement | null> }) {
  const [items, setItems] = useState<OutlineItem[]>([]);

  // MutationObserver instead of threading a `streaming` flag down
  // from the message: the outline rebuilds on every DOM change to
  // its target, so it stays correct during streaming and post-render
  // rewraps. O(n) per rebuild over one message's headings — cheap.
  useEffect(() => {
    const el = target.current;
    if (!el) return;

    const rebuild = () => {
      const headings = Array.from(el.querySelectorAll<HTMLElement>("h1, h2, h3, h4, h5, h6"));
      const next: OutlineItem[] = [];
      for (const h of headings) {
        const text = h.textContent?.trim() ?? "";
        if (!text) continue;
        // Auto-assign an id if rehype didn't (we don't use rehype-slug).
        if (!h.id) h.id = `h-${next.length}-${text.slice(0, 24).replace(/\s+/g, "-")}`;
        next.push({ id: h.id, level: Number(h.tagName.slice(1)), text });
      }
      setItems(next);
    };

    rebuild();
    const obs = new MutationObserver(rebuild);
    obs.observe(el, { childList: true, subtree: true, characterData: true });
    return () => obs.disconnect();
  }, [target]);

  if (items.length < MIN_ITEMS) return null;

  // Indent by heading level relative to the shallowest heading so a
  // message whose top heading is h2 doesn't waste left padding.
  const minLevel = Math.min(...items.map((i) => i.level));

  return (
    // Outer aside spans the message height in the right gutter.
    // Inner div is sticky so the outline tracks scrolling while the
    // message is in view (sticky climbs to find panel-scroll as the
    // scroll ancestor, not the absolute aside above it).
    <aside
      aria-label="Message outline"
      className="hidden xl:block absolute inset-y-0 left-[calc(100%+16px)] w-44"
    >
      <div className="sticky top-4 max-h-[60vh] overflow-y-auto">
        <div className="mb-1.5 font-mono text-[10px] font-semibold uppercase tracking-wider text-fg-faint">
          On this message
        </div>
        <ul className="flex flex-col gap-0.5">
          {items.map((it) => (
            <li key={it.id}>
              <a
                href={`#${it.id}`}
                onClick={(e) => {
                  e.preventDefault();
                  document
                    .getElementById(it.id)
                    ?.scrollIntoView({ block: "start", behavior: "smooth" });
                }}
                style={{ paddingLeft: `${(it.level - minLevel) * 10}px` }}
                className={cn(
                  "block truncate rounded px-1.5 py-0.5 text-[12px] text-fg-muted cursor-pointer transition-colors",
                  "hover:bg-surface-2 hover:text-fg",
                  it.level === minLevel && "font-semibold text-fg-soft",
                )}
                title={it.text}
              >
                {it.text}
              </a>
            </li>
          ))}
        </ul>
      </div>
    </aside>
  );
}
