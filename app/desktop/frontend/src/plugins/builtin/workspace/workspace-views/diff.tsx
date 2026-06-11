// Built-in plugin: "Diff" workspace view — structured per-file diff from
// workspace.getDiff (AUX_API §2.3). With a file selected (Files tab) it
// scopes to that path; otherwise it shows the whole working tree.

import type { FileDiff } from "@/lib/data/queries";
import { DataView, Icon, IconButton } from "@/components/common";
import { DiffView } from "./views/DiffView";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useDiff } from "@/lib/data/queries";
import { useActiveSessionCwd } from "@/lib/agent/useActiveSessionCwd";
import { cn } from "@/lib/utils";
import { gitOffEmpty, isVcsUnavailable, notARepoEmpty } from "./views/vcsGate";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { useServerFeature } from "@/state/runtimeStore";
import { useSessionStore } from "@/state/sessionStore";

function FileSection({ file, showHeader }: { file: FileDiff; showHeader: boolean }) {
  return (
    <section>
      {showHeader && (
        <div className="flex items-center gap-2 bg-surface-2 px-3 py-1.5 font-mono text-[11px] text-fg-muted">
          <span className="truncate">
            {file.previousPath ? `${file.previousPath} → ${file.path}` : file.path}
          </span>
          {file.added !== undefined && <span className="ml-auto text-accent">+{file.added}</span>}
          {file.removed !== undefined && <span className="text-negative">−{file.removed}</span>}
        </div>
      )}
      {file.binary ? (
        <p className="m-0 px-3 py-2 font-mono text-[11.5px] text-fg-faint">Binary file</p>
      ) : (
        <DiffView rows={file.rows} />
      )}
    </section>
  );
}

function DiffViewTab() {
  const gitEnabled = useServerFeature("git");
  const cwd = useActiveSessionCwd();
  const activeFile = useSessionStore((s) => s.activeFile);
  const { data, isLoading, isError, error } = useDiff(
    gitEnabled ? { cwd, path: activeFile || undefined } : undefined,
  );
  const files = data?.files;
  const added = files?.reduce((s, f) => s + (f.added ?? 0), 0) ?? 0;
  const removed = files?.reduce((s, f) => s + (f.removed ?? 0), 0) ?? 0;
  const notARepo = isVcsUnavailable(error);

  const sub = (
    <>
      <span className="text-accent">+{added}</span>
      <span className="mx-1">·</span>
      <span className="text-negative">−{removed}</span>
      <span className="mx-2">·</span>
      <span>{files?.length ?? 0} files</span>
    </>
  );

  return (
    <WorkspaceViewLayout
      icon="file"
      title={activeFile || "Working tree"}
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
    >
      <DataView
        items={gitEnabled ? files : []}
        isLoading={isLoading}
        // A non-repo cwd is an expected state with its own copy, not a failure.
        isError={isError && !notARepo}
        skeletonCount={10}
        empty={!gitEnabled ? gitOffEmpty("diff") : notARepo ? notARepoEmpty("diff") : EMPTY_DIFF}
        error={{
          icon: "diff",
          title: "Couldn't load the diff",
          sub: "The runtime rejected workspace.getDiff — see Diagnostics.",
        }}
      >
        {(fileDiffs) => (
          <div className={cn(data?.truncated && "pb-1")}>
            {fileDiffs.map((f) => (
              <FileSection key={f.path} file={f} showHeader={fileDiffs.length > 1} />
            ))}
            {data?.truncated && (
              <p className="m-0 px-3 py-2 font-mono text-[11px] text-fg-faint">
                Diff truncated at the row limit — narrow to a single file for the rest.
              </p>
            )}
          </div>
        )}
      </DataView>
    </WorkspaceViewLayout>
  );
}

const EMPTY_DIFF = {
  icon: "diff",
  title: "Nothing to compare",
  sub: "The working tree has no uncommitted changes.",
} as const;

export const diffView = defineWorkspaceView({
  id: "diff",
  title: "Diff",
  icon: "diff",
  openByDefault: false,
  order: 0,
  component: DiffViewTab,
});
