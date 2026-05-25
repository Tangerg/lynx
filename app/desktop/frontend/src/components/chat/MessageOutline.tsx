// Floating heading outline for long assistant messages.
//
// Watches its `target` ref for `h1`-`h6` descendants, builds a TOC, and
// scrolls the target heading into view on click. Hides itself when
// fewer than 3 headings are present (short messages don't need
// navigation). Mounts as a sibling next to the message content so it
// shares the surrounding column flow.

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

  // Rebuild the list whenever the message content changes (streaming
  // delta, post-render rewrap). Uses MutationObserver so we don't have
  // to thread the streaming flag down. Cheap: each rebuild is O(n) over
  // a single message's heading nodes.
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
    <aside
      aria-label="Message outline"
      className="hidden xl:block sticky top-4 ml-3 max-h-[60vh] w-44 shrink-0 overflow-y-auto"
    >
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
    </aside>
  );
}
