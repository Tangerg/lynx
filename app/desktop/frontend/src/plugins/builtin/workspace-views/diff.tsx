// Built-in plugin: "Diff" workspace view — renders the diff for the
// currently selected file (kept in useSessionStore.activeFile).

import type { DiffRow } from "@/lib/data/queries";
import { DataView, Icon, IconButton, ScrollArea } from "@/components/common";
import { DiffView } from "@/components/views/DiffView";
import { ViewHeader } from "@/components/views/ViewHeader";
import { useDiff } from "@/lib/data/queries";
import { definePlugin } from "@/plugins/sdk";
import { useSessionStore } from "@/state/sessionStore";

interface DiffStats {
  added: number;
  removed: number;
  lineCount: number;
}

// Real diff metadata isn't on the query result yet, so we derive what we can
// from the rendered rows. Once the data layer surfaces per-file stats, swap
// these helpers for whatever the query returns.
function summarize(rows: DiffRow[] | undefined): DiffStats {
  if (!rows || rows.length === 0) return { added: 0, removed: 0, lineCount: 0 };
  let added = 0;
  let removed = 0;
  let lineCount = 0;
  for (const row of rows) {
    if (row.type === "add") {
      added += 1;
      lineCount += 1;
    } else if (row.type === "del") {
      removed += 1;
    } else if (row.type === "ctx") {
      lineCount += 1;
    }
  }
  return { added, removed, lineCount };
}

function DiffViewTab() {
  const activeFile = useSessionStore((s) => s.activeFile);
  const { data: rows, isLoading } = useDiff();
  const { added, removed, lineCount } = summarize(rows);

  const sub = (
    <>
      <span style={{ color: "var(--color-accent)" }}>+{added}</span>
      <span style={{ margin: "0 4px" }}>·</span>
      <span style={{ color: "var(--color-negative)" }}>−{removed}</span>
      <span style={{ margin: "0 8px", color: "var(--color-text-faint)" }}>·</span>
      <span>{lineCount} lines</span>
    </>
  );

  return (
    <>
      <ViewHeader
        icon="file"
        title={activeFile || "src/api/auth.ts"}
        sub={sub}
        actions={
          <>
            <IconButton title="Revert">
              <Icon name="loop" size={14} />
            </IconButton>
            <IconButton title="Accept">
              <Icon name="check" size={14} />
            </IconButton>
          </>
        }
      />
      <ScrollArea>
        <DataView
          items={rows}
          isLoading={isLoading}
          skeletonCount={10}
          empty={{
            icon: "diff",
            title: "Nothing to compare",
            sub: "Pick a file in the Files tab to see its diff.",
          }}
        >
          {(diffRows) => <DiffView rows={diffRows} />}
        </DataView>
      </ScrollArea>
    </>
  );
}

export const diffView = definePlugin({
  name: "lyra.builtin.view-diff",
  version: "1.0.0",
  setup({ host }) {
    host.workspace.registerView({
      id: "diff",
      title: "Diff",
      icon: "diff",
      openByDefault: false,
      order: 0,
      component: DiffViewTab,
    });
  },
});
