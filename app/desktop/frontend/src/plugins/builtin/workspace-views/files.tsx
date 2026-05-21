// Built-in plugin: "Files" workspace view — the working-tree summary.
//
// Reads the file list from the data-provider registry (lyra.builtin.default-data
// supplies the mock). Selecting a row updates the shared active-file
// state and opens the Diff tab.

import { EmptyState, Icon, IconButton, ScrollArea, SkeletonList } from "@/components/common";
import { FilesChanged } from "@/components/views/FilesChanged";
import { ViewHeader } from "@/components/views/ViewHeader";
import { useFilesChanged } from "@/lib/queries";
import { definePlugin } from "@/plugins/sdk";
import { useUIStore } from "@/state/uiStore";

function FilesView() {
  const activeFile = useUIStore((s) => s.activeFile);
  const setActiveFile = useUIStore((s) => s.setActiveFile);
  const openMainView = useUIStore((s) => s.openMainView);
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
            <IconButton title="Stage all"><Icon name="check" size={14} /></IconButton>
            <IconButton title="More"><Icon name="more" size={14} /></IconButton>
          </>
        }
      />
      <ScrollArea>
        {isLoading ? (
          <SkeletonList count={6} />
        ) : items.length === 0 ? (
          <EmptyState
            icon="check"
            title="Working tree clean"
            sub="No uncommitted changes in the current workspace."
          />
        ) : (
          <FilesChanged
            files={items}
            activePath={activeFile}
            onSelect={(p) => {
              setActiveFile(p);
              openMainView({ id: "diff", title: "Diff", icon: "diff" });
            }}
          />
        )}
      </ScrollArea>
    </>
  );
}

export const filesView = definePlugin({
  name: "lyra.builtin.view-files",
  version: "1.0.0",
  setup({ host }) {
    host.workspace.registerView({
      id: "files",
      title: "Files",
      icon: "filetext",
      openByDefault: false,
      order: 20,
      component: FilesView,
    });
  },
});
