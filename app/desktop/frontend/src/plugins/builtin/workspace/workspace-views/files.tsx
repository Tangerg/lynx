// Built-in plugin: "Files" workspace view — the working-tree summary from
// workspace.listFileChanges (AUX_API §2.2). Selecting a row updates the
// shared active-file state and opens the Diff tab.

import { DataView, Icon, IconButton } from "@/components/common";
import { FilesChanged } from "./views/FilesChanged";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useFilesChanged } from "@/lib/data/queries";
import { errorType, RpcError } from "@/rpc";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { useServerFeature } from "@/state/runtimeStore";
import { useSessionStore } from "@/state/sessionStore";

function FilesView() {
  const gitEnabled = useServerFeature("git");
  const activeFile = useSessionStore((s) => s.activeFile);
  const setActiveFile = useSessionStore((s) => s.setActiveFile);
  const openMainView = useSessionStore((s) => s.openMainView);
  const { data: files, isLoading, isError, error } = useFilesChanged({ enabled: gitEnabled });
  const items = files ?? [];
  const notARepo = error instanceof RpcError && errorType(error.data) === "vcs_unavailable";

  return (
    <WorkspaceViewLayout
      icon="filetext"
      titleStrong
      title="Working tree"
      sub={`${items.length} files · uncommitted`}
      actions={
        <>
          <IconButton title="Stage all">
            <Icon name="check" size={14} />
          </IconButton>
          <IconButton title="More">
            <Icon name="more" size={14} />
          </IconButton>
        </>
      }
    >
      <DataView
        items={gitEnabled ? items : []}
        isLoading={isLoading}
        // AUX_API §3: a non-repo cwd is an expected state, not a failure.
        isError={isError && !notARepo}
        skeletonCount={6}
        empty={
          !gitEnabled
            ? {
                icon: "filetext",
                title: "Git not available",
                sub: "This runtime has no git binary on its PATH.",
              }
            : notARepo
              ? {
                  icon: "filetext",
                  title: "Not a git repository",
                  sub: "The session's working directory is not under version control.",
                }
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
              openMainView({ id: "diff", title: "Diff", icon: "diff" });
            }}
          />
        )}
      </DataView>
    </WorkspaceViewLayout>
  );
}

export const filesView = defineWorkspaceView({
  id: "files",
  title: "Files",
  icon: "filetext",
  openByDefault: false,
  order: 20,
  component: FilesView,
});
