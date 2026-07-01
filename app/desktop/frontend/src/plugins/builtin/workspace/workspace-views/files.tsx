// Built-in plugin: "Files" workspace view — the working-tree summary from
// workspace.listFileChanges (AUX_API §2.2). Selecting a row updates the
// shared active-file state and opens the Diff tab.

import { DataView } from "@/components/common";
import { useT } from "@/lib/i18n";
import { FilesChanged } from "./views/FilesChanged";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useActiveSessionCwd } from "@/plugins/builtin/agent/public/session";
import { useWorkspaceFileChanges } from "@/plugins/builtin/workspace/application/workspaceData";
import {
  openWorkspaceDiffForFile,
  useActiveWorkspaceFile,
} from "@/plugins/builtin/workspace/public/navigation";
import { gitOffEmpty, isVcsUnavailable, notARepoEmpty } from "./views/vcsGate";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { useWorkspaceCapability } from "@/plugins/builtin/workspace/application/workspaceCapabilities";

function FilesView() {
  const t = useT();
  const gitEnabled = useWorkspaceCapability("git");
  const cwd = useActiveSessionCwd();
  const activeFile = useActiveWorkspaceFile();
  // Scoped to the ACTIVE session's cwd (undefined = serve dir fallback);
  // disabled entirely while the git capability is off.
  const {
    data: files,
    isLoading,
    isError,
    error,
  } = useWorkspaceFileChanges(gitEnabled ? { cwd } : undefined);
  const items = files ?? [];
  const notARepo = isVcsUnavailable(error);

  return (
    <WorkspaceViewLayout
      icon="filetext"
      titleStrong
      title="files.title"
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
                  title: t("files.empty.title"),
                  sub: t("files.empty.sub"),
                }
        }
      >
        {(rows) => (
          <FilesChanged files={rows} activePath={activeFile} onSelect={openWorkspaceDiffForFile} />
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
