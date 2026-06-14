// Floating heading outline for assistant messages. Anchored to the
// gutter outside the 760px content column so it never compresses
// content width. Hides itself below MIN_ITEMS (short messages don't
// need a TOC) and when the chat panel doesn't have a 192px right
// gutter to host the outline (container query, not viewport — sidebar
// mode + window width together drive the available width).
//
// MessageBlock skips mounting this while the message is streaming, so
// the per-token MutationObserver path below only runs against a settled
// message — but the rAF coalesce + structural-equality bail stay as
// defense-in-depth in case a future caller mounts mid-stream.

import type { RefObject } from "react";
import { useEffect, useState } from "react";
import { cn } from "@/lib/utils";
import { useT } from "@/lib/i18n";

interface OutlineItem {
  id: string;
  level: number;
  text: string;
}

const MIN_ITEMS = 3;

function sameItems(a: OutlineItem[], b: OutlineItem[]): boolean {
  if (a.length !== b.length) return false;
  for (let i = 0; i < a.length; i++) {
    // Bound-checked above; non-null asserts are safe here.
    const ai = a[i]!;
    const bi = b[i]!;
    if (ai.id !== bi.id || ai.level !== bi.level || ai.text !== bi.text) return false;
  }
  return true;
}

export function MessageOutline({
  target,
  scopeId,
}: {
  target: RefObject<HTMLElement | null>;
  /** Owning message id — namespaces the auto-assigned heading ids. Without it
   *  two messages with the same heading at the same position ("Summary",
   *  "Plan"…) mint identical DOM ids and getElementById resolves every
   *  outline click to the FIRST message in the stream. */
  scopeId: string;
}) {
  const t = useT();
  const [items, setItems] = useState<OutlineItem[]>([]);

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
        if (!h.id) h.id = `h-${scopeId}-${next.length}-${text.slice(0, 24).replace(/\s+/g, "-")}`;
        next.push({ id: h.id, level: Number(h.tagName.slice(1)), text });
      }
      setItems((prev) => (sameItems(prev, next) ? prev : next));
    };

    // rAF-coalesce so a burst of mutations (e.g. a streaming render
    // dumping many text nodes in the same task) collapses to one rebuild
    // per frame instead of one per mutation.
    let raf = 0;
    const schedule = () => {
      if (raf) return;
      raf = requestAnimationFrame(() => {
        raf = 0;
        rebuild();
      });
    };

    rebuild();
    const obs = new MutationObserver(schedule);
    obs.observe(el, { childList: true, subtree: true, characterData: true });
    return () => {
      obs.disconnect();
      if (raf) cancelAnimationFrame(raf);
    };
  }, [target, scopeId]);

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
      aria-label={t("message.outline.label")}
      // Threshold: chat needs ≥ 760 + 2 × (16 + 176) = 1144px panel
      // width to host the outline. With an expanded 248px sidebar +
      // 16px gaps that means viewport ≥ ~1408px. Using a viewport
      // media query (rather than a container query) sidesteps the
      // `container-type: inline-size` / use-stick-to-bottom scroll
      // anchoring issue that surfaced during streaming.
      className="hidden min-[1408px]:block absolute inset-y-0 left-[calc(100%+16px)] w-44"
    >
      <div className="sticky top-4 max-h-[60vh] overflow-y-auto">
        <div className="mb-1.5 font-mono text-[10px] font-semibold text-fg-faint">
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
                  "block truncate rounded px-1.5 py-0.5 text-[12px] text-fg-muted transition-colors",
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
