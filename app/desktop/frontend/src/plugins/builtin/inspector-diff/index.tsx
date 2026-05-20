import { Icon, IconButton, ScrollArea } from "@/components/common";
import { DiffView } from "@/components/inspector/DiffView";
import { useInspector } from "@/components/inspector/InspectorContext";
import { useDiff } from "@/lib/queries";
import { definePlugin } from "@/plugins/sdk";

function DiffTab() {
  const { activeFile } = useInspector();
  const { data: rows } = useDiff();

  // These were hardcoded in the old InspectorPanel layer; eventually they
  // should come from the same data layer as the diff itself.
  const added = 47;
  const removed = 31;
  const lineCount = 247;
  const language = "TypeScript";

  return (
    <>
      <div className="insp-head">
        <div className="ficon"><Icon name="file" size={14} /></div>
        <div style={{ minWidth: 0 }}>
          <div className="ftitle">{activeFile || "src/api/auth.ts"}</div>
          <div className="fsub">
            <span style={{ color: "var(--color-accent)" }}>+{added}</span>
            <span style={{ margin: "0 4px" }}>·</span>
            <span style={{ color: "var(--color-negative)" }}>−{removed}</span>
            <span style={{ margin: "0 8px", color: "var(--color-text-faint)" }}>·</span>
            <span>{lineCount} lines · {language}</span>
          </div>
        </div>
        <div style={{ display: "flex", gap: 4 }}>
          <IconButton title="Revert"><Icon name="loop" size={14} /></IconButton>
          <IconButton title="Accept"><Icon name="check" size={14} /></IconButton>
        </div>
      </div>
      <ScrollArea><DiffView rows={rows ?? []} /></ScrollArea>
    </>
  );
}

export default definePlugin({
  name: "lyra.builtin.inspector-diff",
  version: "1.0.0",
  setup({ host }) {
    host.inspector.registerTab({
      id: "diff",
      label: "Diff",
      icon: "diff",
      order: 0,
      component: DiffTab,
    });
  },
});
