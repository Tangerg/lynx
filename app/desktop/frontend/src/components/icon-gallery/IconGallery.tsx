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
    <div className="icon-gallery">
      <div className="ig-head">
        <div>
          <div className="ig-title">@lobehub/icons</div>
          <div className="ig-sub">{rawToc.length} icons · brands across LLM models, providers, and apps</div>
        </div>
        <div className="ig-search">
          <Icon name="search" size={13} />
          <input
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Filter by name…"
          />
          {query && (
            <button className="ig-clear" onClick={() => setQuery("")} title="Clear">
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
            <section key={g} className="ig-section">
              <header className="ig-section-head">
                <span>{GROUP_TITLES[g]}</span>
                <span className="ig-section-count">{list.length}</span>
              </header>
              <div className="ig-grid">
                {list.map((entry) => (
                  <IconCard key={entry.id} entry={entry} />
                ))}
              </div>
            </section>
          );
        })}
        {items.length === 0 && (
          <div className="ig-empty">No icons match "{query}".</div>
        )}
      </ScrollArea>
    </div>
  );
}

function IconCard({ entry }: { entry: (typeof rawToc)[number] }) {
  const Component = IconMap[entry.id];
  return (
    <div className="ig-card" title={`${entry.fullTitle} — ${entry.id}`}>
      <div className="ig-glyph">
        {Component ? <Component size={28} /> : <span className="ig-missing">?</span>}
      </div>
      <div className="ig-name">{entry.fullTitle}</div>
      <div className="ig-meta">
        <span
          className="ig-swatch"
          style={{ background: entry.color }}
          title={entry.color}
        />
        <code className="ig-id">{entry.id}</code>
      </div>
    </div>
  );
}
