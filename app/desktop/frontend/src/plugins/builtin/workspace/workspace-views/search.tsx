// Built-in workspace view: "Search" — user-facing workspace.files.search over the
// active session's cwd. Until now grep only powered tool-card previews; this
// gives the user a direct regex search entry. Debounced live query; results
// grouped by file; server truncation surfaced honestly (§7.5 no-silent-caps:
// total > matches.length means "narrow the query", never "that's all").

import { useState } from "react";
import { useDebouncedValue } from "@tanstack/react-pacer";
import { DataView, Pressable, SearchField } from "@/ui";
import { useT } from "@/lib/i18n";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { useWorkspaceGrep } from "@/plugins/builtin/workspace/application/workspaceQueries";
import {
  WORKSPACE_SEARCH_MATCH_LIMIT,
  workspaceSearchSubtext,
  workspaceSearchViewModel,
} from "@/plugins/builtin/workspace/application/searchViewModel";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { openWorkspaceFile } from "@/plugins/builtin/workspace/public/navigation";

function SearchTab() {
  const t = useT();
  const workspace = useActiveSessionWorkspace();
  const [input, setInput] = useState("");
  // Debounce keystrokes so each distinct query hits the backend once — every
  // params object is its own react-query cache entry.
  const [query] = useDebouncedValue(input.trim(), { wait: 300 });
  const { data, isLoading, isError } = useWorkspaceGrep(
    query && workspace.status === "ready"
      ? { query, cwd: workspace.cwd, limit: WORKSPACE_SEARCH_MATCH_LIMIT }
      : undefined,
  );
  const view = workspaceSearchViewModel(data);

  return (
    <WorkspaceViewLayout
      icon="search"
      titleStrong
      title="search.title"
      sub={workspaceSearchSubtext(t, view) ?? t("search.noMatches")}
      scrollClassName="py-1"
    >
      <div className="px-4 pt-1 pb-2">
        <SearchField
          font="mono"
          value={input}
          onValueChange={setInput}
          placeholder={t("search.placeholder")}
          aria-label={t("search.aria")}
          spellCheck={false}
        />
      </div>
      {query === "" ? null : (
        <DataView
          items={data ? view.groups : undefined}
          isLoading={isLoading || workspace.status === "resolving"}
          isError={isError}
          skeletonCount={4}
          empty={{
            icon: "search",
            title: t("search.empty.title"),
            sub: t("search.empty.sub"),
            size: "compact",
          }}
        >
          {(groups) => (
            <div className="flex flex-col pb-2">
              {groups.map((group) => (
                <div key={group.path} className="px-4 py-1.5">
                  <div className="truncate font-mono text-ui-sm font-semibold text-fg">
                    {group.path}
                    <span className="ml-1.5 font-normal text-fg-faint">{group.matchCount}</span>
                  </div>
                  <div className="mt-0.5 flex flex-col">
                    {group.matches.map((m) => (
                      <Pressable
                        key={m.lineNumber}
                        onClick={() => openWorkspaceFile(group.path, m.lineNumber)}
                        className="grid w-full grid-cols-[44px_minmax(0,1fr)] gap-2 rounded-xs py-px pr-1 font-mono text-ui-md leading-body transition-colors hover:bg-hover"
                      >
                        <span className="text-right text-ui-sm text-fg-faint select-none">
                          {m.lineNumber}
                        </span>
                        <span className="truncate text-fg-soft" title={m.text}>
                          {m.text}
                        </span>
                      </Pressable>
                    ))}
                  </div>
                </div>
              ))}
              {view.overflowCount > 0 && (
                <div className="px-4 py-2 text-ui-sm text-fg-faint">
                  … {t("search.overflow", { count: view.overflowCount })}
                </div>
              )}
            </div>
          )}
        </DataView>
      )}
    </WorkspaceViewLayout>
  );
}

export const searchView = defineWorkspaceView({
  id: "search",
  title: "workspace.view.title.search",
  icon: "search",
  order: 10,
  splittable: true,
  component: SearchTab,
});
