// Built-in plugin: "Files" workspace view — the working-tree summary from
// workspace.listFileChanges (AUX_API §2.2). Selecting a row updates the
// shared active-file state and opens the Diff tab.

import { DataView } from "@/components/common";
import { FilesChanged } from "./views/FilesChanged";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useFilesChanged } from "@/lib/data/queries";
import { useActiveSessionCwd } from "@/lib/agent/useActiveSession";
import { gitOffEmpty, isVcsUnavailable, notARepoEmpty } from "./views/vcsGate";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { useServerFeature } from "@/state/runtimeStore";
import { useSessionStore } from "@/state/sessionStore";

function FilesView() {
  const gitEnabled = useServerFeature("git");
  const cwd = useActiveSessionCwd();
  const activeFile = useSessionStore((s) => s.activeFile);
  const setActiveFile = useSessionStore((s) => s.setActiveFile);
  const openMainView = useSessionStore((s) => s.openMainView);
  // Scoped to the ACTIVE session's cwd (undefined = serve dir fallback);
  // disabled entirely while the git capability is off.
  const {
    data: files,
    isLoading,
    isError,
    error,
  } = useFilesChanged(gitEnabled ? { cwd } : undefined);
  const items = files ?? [];
  const notARepo = isVcsUnavailable(error);

  return (
    <WorkspaceViewLayout
      icon="filetext"
      titleStrong
      title="Working tree"
      sub={`${items.length} files · uncommitted`}
    >
      <DataView
        items={gitEnabled ? items : []}
        isLoading={isLoading}
        // AUX_API §3: a non-repo cwd is an expected state, not a failure.
        isError={isError && !notARepo}
        skeletonCount={6}
        empty={
          !gitEnabled
            ? gitOffEmpty("filetext")
            : notARepo
              ? notARepoEmpty("filetext")
              : {
                  icon: "check",
                  title: "Working tree clean",
                  sub: "No uncommitted changes in the current workspace.",
                }
        }
      >
        {(rows) => (
          <FilesChanged
            files={rows}
            activePath={activeFile}
            onSelect={(p) => {
              setActiveFile(p);
              openMainView({ id: "diff", title: "workspace.view.title.diff", icon: "diff" });
            }}
          />
        )}
      </DataView>
    </WorkspaceViewLayout>
  );
}

export const filesView = defineWorkspaceView({
  id: "files",
  title: "workspace.view.title.files",
  icon: "filetext",
  openByDefault: false,
  order: 20,
  splittable: true,
  component: FilesView,
});
