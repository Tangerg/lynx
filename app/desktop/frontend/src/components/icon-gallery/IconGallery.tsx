// IconGallery — browses every brand in @lobehub/icons.
//
// The `.Avatar` / `.Text` variants are off-limits because they pull in
// IconAvatar / IconText from `features/`, which require `@lobehub/ui`
// and `antd` — neither of which we ship.

import { useMemo, useState } from "react";
import { Icon, ScrollArea } from "@/components/common";
import { IconMap, rawToc } from "./iconMap";

type GroupKey = "model" | "provider" | "application";

const GROUP_TITLES: Record<GroupKey, string> = {
  model:       "Models",
  provider:    "Providers",
  application: "Applications",
};

export function IconGallery() {
  const [query, setQuery] = useState("");

  // Filter once per query — cheap (≤300 entries).
  const items = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return rawToc;
    return rawToc.filter((e) =>
      e.fullTitle.toLowerCase().includes(q) || e.id.toLowerCase().includes(q),
    );
  }, [query]);

  // Group by `group` and keep alphabetical order inside each.
  const grouped = useMemo(() => {
    const buckets: Record<GroupKey, typeof rawToc> = {
      model: [], provider: [], application: [],
    };
    for (const e of items) {
      if (e.group in buckets) buckets[e.group as GroupKey].push(e);
    }
    for (const k of Object.keys(buckets) as GroupKey[]) {
      buckets[k].sort((a, b) => a.fullTitle.localeCompare(b.fullTitle));
    }
    return buckets;
  }, [items]);

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="flex items-center justify-between gap-4 px-5 py-4">
        <div>
          <div className="text-[15px] font-semibold tracking-[-0.01em]">@lobehub/icons</div>
          <div className="mt-0.5 text-[11.5px] text-fg-faint">
            {rawToc.length} icons · brands across LLM models, providers, and apps
          </div>
        </div>
        <div className="relative flex w-60 items-center gap-1.5 rounded-md border border-transparent bg-surface-2 px-2.5 py-1 transition-colors duration-150 focus-within:border-[color-mix(in_srgb,var(--color-accent)_35%,var(--color-line))]">
          <Icon name="search" size={13} className="shrink-0 text-fg-faint" />
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Filter by name…"
            className="flex-1 border-0 bg-transparent text-[12px] text-fg font-inherit outline-none placeholder:text-fg-faint"
          />
          {query && (
            <button
              type="button"
              onClick={() => setQuery("")}
              title="Clear"
              className="grid h-5.5 w-5.5 place-items-center rounded-xs border-0 bg-transparent p-0 text-fg-faint cursor-pointer transition-[background,color,transform] duration-150 hover:bg-surface-3 hover:text-fg active:scale-90"
            >
              <Icon name="x" size={11} />
            </button>
          )}
        </div>
      </div>

      <ScrollArea>
        {(Object.keys(grouped) as GroupKey[]).map((g) => {
          const list = grouped[g];
          if (list.length === 0) return null;
          return (
            <section key={g} className="px-5 pt-4.5 pb-3">
              <header className="flex items-baseline justify-between pb-2.5 font-mono text-[11px] font-semibold text-fg-faint tracking-normal">
                <span>{GROUP_TITLES[g]}</span>
                <span className="font-mono text-fg-muted tabular-nums">{list.length}</span>
              </header>
              <div className="grid gap-2 [grid-template-columns:repeat(auto-fill,minmax(120px,1fr))]">
                {list.map((entry) => (
                  <IconCard key={entry.id} entry={entry} />
                ))}
              </div>
            </section>
          );
        })}
        {items.length === 0 && (
          <div className="px-5 py-16 text-center text-[12px] text-fg-faint">
            No icons match "{query}".
          </div>
        )}
      </ScrollArea>
    </div>
  );
}

function IconCard({ entry }: { entry: (typeof rawToc)[number] }) {
  const Component = IconMap[entry.id];
  return (
    <div
      title={`${entry.fullTitle} — ${entry.id}`}
      className="flex flex-col items-center gap-1.5 rounded-lg border border-line bg-surface px-2.5 pt-3.5 pb-2.5 cursor-default transition-[border-color,transform] duration-150 hover:border-[color-mix(in_srgb,var(--color-accent)_30%,var(--color-line))] hover:-translate-y-px"
    >
      <div className="grid h-11 w-11 place-items-center rounded-md bg-surface-2 text-fg">
        {Component ? <Component size={28} /> : <span className="font-mono text-fg-faint">?</span>}
      </div>
      <div className="max-w-full truncate text-center text-[11.5px] font-medium text-fg">
        {entry.fullTitle}
      </div>
      <div className="flex items-center gap-1.5 text-[10px]">
        <span
          title={entry.color}
          className="h-2 w-2 rounded-full border border-[color-mix(in_srgb,var(--color-text)_10%,transparent)]"
          style={{ background: entry.color }}
        />
        <code className="font-mono text-[10px] text-fg-faint tracking-[-0.005em]">{entry.id}</code>
      </div>
    </div>
  );
}
