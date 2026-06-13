import { useState } from "react";
import { DataView, Icon } from "@/components/common";
import { useActiveSessionCwd } from "@/lib/agent/useActiveSession";
import { useListFiles, useReadFile } from "@/lib/data/queries";
import { FileTree } from "./views/FileTree";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { defineWorkspaceView } from "./defineWorkspaceView";

// The workspace file-tree browser (B8/G12). Lazy tree of the active session's
// cwd; selecting a file swaps to a plain-text viewer (workspace.readFile, capped
// + self-describing-truncated). Not feature-gated — listFiles/readFile are basic
// reads — but a pre-B8 runtime errors the query, which DataView surfaces.

function FileViewer({ path, cwd, onBack }: { path: string; cwd?: string; onBack: () => void }) {
  const { data, isLoading, isError } = useReadFile({ path, cwd });
  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <button
        type="button"
        onClick={onBack}
        className="flex items-center gap-1.5 px-3 py-2 text-left font-mono text-[12px] text-fg-muted hover:text-fg"
      >
        <Icon name="chevron-down" size={12} className="shrink-0 -rotate-90" />
        <span className="truncate">{path}</span>
      </button>
      {isLoading ? (
        <div className="px-3 text-[12px] text-fg-faint">Loading…</div>
      ) : isError || !data ? (
        <div className="px-3 text-[12px] text-negative">Couldn't read this file.</div>
      ) : (
        <pre className="whitespace-pre-wrap break-words px-3 pb-3 font-mono text-[12px] leading-relaxed text-fg">
          {data.content}
          {data.truncated ? "\n\n… truncated (file too large to show whole)" : ""}
        </pre>
      )}
    </div>
  );
}

function ExplorerView() {
  const cwd = useActiveSessionCwd();
  const [selected, setSelected] = useState<string | null>(null);
  const { data: roots, isLoading, isError } = useListFiles({ cwd });

  return (
    <WorkspaceViewLayout icon="folder" titleStrong title="Explorer">
      {selected ? (
        <FileViewer path={selected} cwd={cwd} onBack={() => setSelected(null)} />
      ) : (
        <DataView
          items={roots}
          isLoading={isLoading}
          isError={isError}
          skeletonCount={8}
          empty={{ icon: "folder", title: "Nothing to browse", sub: "No files in this workspace." }}
        >
          {(rows) => <FileTree entries={rows} cwd={cwd} onSelectFile={setSelected} />}
        </DataView>
      )}
    </WorkspaceViewLayout>
  );
}

export const fileTreeView = defineWorkspaceView({
  id: "explorer",
  title: "Explorer",
  icon: "folder",
  openByDefault: false,
  order: 22,
  splittable: true,
  component: ExplorerView,
});
