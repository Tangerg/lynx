import { Icon, IconButton, ScrollArea } from "@/components/common";
import { FilesChanged } from "@/components/inspector/FilesChanged";
import { useInspector } from "@/components/inspector/InspectorPanel";
import { useFilesChanged } from "@/lib/queries";
import { definePlugin } from "@/plugins/sdk";

function FilesTab() {
  const { activeFile, onSelectFile, onSwitchTab } = useInspector();
  const { data: files } = useFilesChanged();
  const items = files ?? [];

  return (
    <>
      <div className="insp-head">
        <div className="ficon"><Icon name="filetext" size={14} /></div>
        <div style={{ minWidth: 0 }}>
          <div className="ftitle" style={{ fontFamily: "var(--font-ui)", fontSize: 13, fontWeight: 700 }}>
            Working tree
          </div>
          <div className="fsub">{items.length} files · uncommitted</div>
        </div>
        <div style={{ display: "flex", gap: 4 }}>
          <IconButton title="Stage all"><Icon name="check" size={14} /></IconButton>
          <IconButton title="More"><Icon name="more" size={14} /></IconButton>
        </div>
      </div>
      <ScrollArea>
        <FilesChanged
          files={items}
          activePath={activeFile}
          onSelect={(p) => { onSelectFile(p); onSwitchTab("diff"); }}
        />
      </ScrollArea>
    </>
  );
}

function useFilesBadge(): number | undefined {
  const { data } = useFilesChanged();
  return data?.length;
}

export default definePlugin({
  name: "lyra.builtin.inspector-files",
  version: "1.0.0",
  setup({ host }) {
    host.inspector.registerTab({
      id: "files",
      label: "Files",
      icon: "filetext",
      order: 20,
      useBadge: useFilesBadge,
      component: FilesTab,
    });
  },
});
