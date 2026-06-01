// Built-in plugin: "Files" workspace view — the working-tree summary.
//
// Reads the file list from the data-provider registry (lyra.builtin.default-data
// supplies the mock). Selecting a row updates the shared active-file
// state and opens the Diff tab.

import { DataView, Icon, IconButton, ScrollArea } from "@/components/common";
import { FilesChanged } from "./views/FilesChanged";
import { ViewHeader } from "./views/ViewHeader";
import { useFilesChanged } from "@/lib/data/queries";
import { defineWorkspaceView } from "./defineWorkspaceView";
import { useSessionStore } from "@/state/sessionStore";

function FilesView() {
  const activeFile = useSessionStore((s) => s.activeFile);
  const setActiveFile = useSessionStore((s) => s.setActiveFile);
  const openMainView = useSessionStore((s) => s.openMainView);
  const { data: files, isLoading } = useFilesChanged();
  const items = files ?? [];

  return (
    <>
      <ViewHeader
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
      />
      <ScrollArea>
        <DataView
          items={items}
          isLoading={isLoading}
          skeletonCount={6}
          empty={{
            icon: "check",
            title: "Working tree clean",
            sub: "No uncommitted changes in the current workspace.",
          }}
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
      </ScrollArea>
    </>
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
