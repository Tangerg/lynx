// IconGallery — browses every brand in @lobehub/icons.
//
// The `.Avatar` / `.Text` variants are off-limits because they pull in
// IconAvatar / IconText from `features/`, which require `@lobehub/ui`
// and `antd` — neither of which we ship.

import { useMemo, useState } from "react";
import { ScrollArea, SearchField } from "@/ui";
import { useT } from "@/lib/i18n";
import { IconMap, rawToc } from "./iconMap";

type GroupKey = "model" | "provider" | "application";

const GROUP_TITLES: Record<GroupKey, string> = {
  model: "Models",
  provider: "Providers",
  application: "Applications",
};

export function IconGallery() {
  const t = useT();
  const [query, setQuery] = useState("");

  // Filter once per query — cheap (≤300 entries).
  const items = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return rawToc;
    return rawToc.filter(
      (e) => e.fullTitle.toLowerCase().includes(q) || e.id.toLowerCase().includes(q),
    );
  }, [query]);

  // Group by `group` and keep alphabetical order inside each.
  const grouped = useMemo(() => {
    const buckets: Record<GroupKey, typeof rawToc> = {
      model: [],
      provider: [],
      application: [],
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
          <div className="text-display-sm font-medium text-fg">@lobehub/icons</div>
          <div className="mt-1 text-ui-md text-fg-muted">
            {t("iconGallery.subtitle", { count: rawToc.length })}
          </div>
        </div>
        <SearchField
          value={query}
          onValueChange={setQuery}
          aria-label={t("iconGallery.filterLabel")}
          placeholder={t("iconGallery.filterPlaceholder")}
          onClear={() => setQuery("")}
          clearLabel={t("iconGallery.clear")}
          className="w-60"
        />
      </div>

      <ScrollArea>
        {(Object.keys(grouped) as GroupKey[]).map((g) => {
          const list = grouped[g];
          if (list.length === 0) return null;
          return (
            <section key={g} className="px-5 pt-4.5 pb-3">
              <header className="flex items-baseline justify-between pb-2.5 font-mono text-ui-sm font-medium tracking-normal text-fg-muted">
                <span>{GROUP_TITLES[g]}</span>
                <span className="font-mono text-fg-faint">{list.length}</span>
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
          <div className="px-5 py-16 text-center text-ui-md text-fg-faint">
            {t("iconGallery.empty", { q: query })}
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
      className="flex cursor-default flex-col items-center gap-1.5 rounded-md bg-card px-2.5 pb-2.5 pt-3.5 transition-colors duration-[var(--dur-fast)] hover:bg-hover"
    >
      <div className="grid h-11 w-11 place-items-center rounded-md bg-surface-2 text-fg">
        {Component ? <Component size={28} /> : <span className="font-mono text-fg-faint">?</span>}
      </div>
      <div className="max-w-full truncate text-center text-ui-sm font-medium text-fg">
        {entry.fullTitle}
      </div>
      <div className="flex items-center gap-1.5 text-ui-xs">
        <span
          title={entry.color}
          className="h-2 w-2 rounded-full border-[0.5px] border-field"
          style={{ background: entry.color }}
        />
        <code className="font-mono text-ui-xs text-fg-muted">{entry.id}</code>
      </div>
    </div>
  );
}
