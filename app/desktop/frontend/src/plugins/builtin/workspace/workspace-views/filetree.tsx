import { DataView } from "@/ui";
import { useT } from "@/lib/i18n";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { isUnsupportedMethod } from "@/lib/rpcErrors";
import { useWorkspaceListFiles } from "@/plugins/builtin/workspace/application/workspaceQueries";
import { FileTree } from "./views/FileTree";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { defineWorkspaceView } from "./defineWorkspaceView";
import {
  openWorkspaceFile,
  useWorkspaceFileViewer,
} from "@/plugins/builtin/workspace/public/navigation";

// The workspace file-tree browser (B8/G12). Lazy tree of the active session's
// cwd. The explorer remains navigation: selecting a file opens the dedicated
// preview tab instead of replacing the tree with a mobile-style drill-down.

function ExplorerView() {
  const t = useT();
  const workspace = useActiveSessionWorkspace();
  const cwd = workspace.status === "ready" ? workspace.cwd : undefined;
  const viewer = useWorkspaceFileViewer();
  const {
    data: roots,
    isLoading,
    isError,
    error,
  } = useWorkspaceListFiles(workspace.status === "ready" ? { cwd } : undefined);

  return (
    <WorkspaceViewLayout icon="folder" titleStrong title="filetree.title">
      <DataView
        items={roots}
        isLoading={isLoading || workspace.status === "resolving"}
        isError={isError}
        // A runtime without workspace.files.list errors the query —
        // show a calm "unavailable here" state, not the generic load error.
        error={
          isUnsupportedMethod(error)
            ? {
                icon: "folder",
                title: t("runtime.unsupported.title"),
                sub: t("runtime.unsupported.sub"),
              }
            : undefined
        }
        skeletonCount={8}
        empty={{ icon: "folder", title: t("filetree.empty.title"), sub: t("filetree.empty.sub") }}
      >
        {(rows) => (
          <FileTree
            entries={rows}
            cwd={cwd}
            selectedPath={viewer?.path}
            onSelectFile={openWorkspaceFile}
          />
        )}
      </DataView>
    </WorkspaceViewLayout>
  );
}

export const fileTreeView = defineWorkspaceView({
  id: "explorer",
  title: "workspace.view.title.filetree",
  icon: "folder",
  order: 20,
  splittable: true,
  component: ExplorerView,
});
