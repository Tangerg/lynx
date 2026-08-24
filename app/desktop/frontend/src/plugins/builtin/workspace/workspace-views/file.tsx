// Built-in plugin: "File" workspace view — renders a bounded file window at a
// target line, opened by a clickable file:line reference in the conversation.

import { DataView, FilePath } from "@/ui";
import { useT } from "@/lib/i18n";
import { FileView } from "./views/FileView";
import { WorkspaceViewLayout } from "./views/WorkspaceViewLayout";
import { useActiveSessionWorkspace } from "@/plugins/builtin/agent/public/session";
import { useWorkspaceReadFile } from "@/plugins/builtin/workspace/application/workspaceQueries";
import { useWorkspaceFileViewer } from "@/plugins/builtin/workspace/public/navigation";
import { defineWorkspaceView } from "./defineWorkspaceView";

const targetWindowRadius = 200;

function FileViewTab() {
  const t = useT();
  const workspace = useActiveSessionWorkspace();
  const cwd = workspace.status === "ready" ? workspace.cwd : undefined;
  const viewer = useWorkspaceFileViewer();
  const targetLine = viewer?.line ?? 0;
  const { data, isLoading, isError } = useWorkspaceReadFile(
    viewer && workspace.status === "ready" && cwd !== undefined
      ? {
          cwd,
          path: viewer.path,
          ...(targetLine > 0
            ? {
                startLine: Math.max(1, targetLine - targetWindowRadius),
                endLine: targetLine + targetWindowRadius,
              }
            : {}),
        }
      : undefined,
  );

  const sub = data ? (
    <span>
      {t("file.lines", { count: data.totalLines })}
      {data.truncated && ` · ${t("file.truncated")}`}
    </span>
  ) : undefined;

  return (
    <WorkspaceViewLayout
      icon="filetext"
      title={viewer?.path || t("file.empty.title")}
      dockIdentity={viewer ? <FilePath path={viewer.path} /> : undefined}
      sub={sub}
    >
      <DataView
        items={data ? [data] : []}
        isLoading={isLoading || (Boolean(viewer) && workspace.status === "resolving")}
        isError={isError}
        skeletonCount={12}
        empty={{ icon: "filetext", title: t("file.empty.title"), sub: t("file.empty.sub") }}
        error={{ icon: "filetext", title: t("file.error.title"), sub: t("file.error.sub") }}
      >
        {(items) => (
          <FileView
            path={viewer?.path ?? ""}
            content={items[0]!.content}
            startLine={items[0]!.startLine}
            targetLine={targetLine}
          />
        )}
      </DataView>
    </WorkspaceViewLayout>
  );
}

export const fileView = defineWorkspaceView({
  id: "file",
  title: "workspace.view.title.file",
  icon: "filetext",
  order: 25,
  splittable: true,
  component: FileViewTab,
});
