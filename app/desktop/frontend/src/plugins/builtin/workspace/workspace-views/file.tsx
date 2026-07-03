// Built-in plugin: "File" workspace view — renders a file's full contents
// (workspace.readFile) at a target line, opened by a clickable file:line
// reference in the conversation.

import { DataView } from "@/components/common";
import { useT } from "@/lib/i18n";
import { FileView } from "./views/FileView";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useActiveSessionCwd } from "@/plugins/builtin/agent/public/session";
import { useWorkspaceReadFile } from "@/plugins/builtin/workspace/application/workspaceData";
import { useWorkspaceFileViewer } from "@/plugins/builtin/workspace/public/navigation";
import { defineWorkspaceView } from "./defineWorkspaceView";

function FileViewTab() {
  const t = useT();
  const cwd = useActiveSessionCwd();
  const viewer = useWorkspaceFileViewer();
  const { data, isLoading, isError } = useWorkspaceReadFile(
    viewer && cwd !== undefined ? { cwd, path: viewer.path } : undefined,
  );

  const sub = data ? (
    <span>
      {t("file.lines", { count: data.totalLines })}
      {data.truncated && ` · ${t("file.truncated")}`}
    </span>
  ) : undefined;

  return (
    <WorkspaceViewLayout icon="filetext" title={viewer?.path || t("file.empty.title")} sub={sub}>
      <DataView
        items={data ? [data] : []}
        isLoading={isLoading}
        isError={isError}
        skeletonCount={12}
        empty={{ icon: "filetext", title: t("file.empty.title"), sub: t("file.empty.sub") }}
        error={{ icon: "filetext", title: t("file.error.title"), sub: t("file.error.sub") }}
      >
        {(items) => <FileView content={items[0]!.content} targetLine={viewer?.line ?? 0} />}
      </DataView>
    </WorkspaceViewLayout>
  );
}

export const fileView = defineWorkspaceView({
  id: "file",
  title: "workspace.view.title.file",
  icon: "filetext",
  order: 1,
  splittable: true,
  component: FileViewTab,
});
