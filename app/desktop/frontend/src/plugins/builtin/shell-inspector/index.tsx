// Built-in plugin: fills the "app.inspector" slot with the InspectorPanel
// (itself a pure router over plugin-contributed inspector tabs).

import { InspectorPanel } from "@/components/inspector/InspectorPanel";
import { definePlugin } from "@/plugins/sdk";
import { useUIStore } from "@/state/uiStore";

function ShellInspector() {
  const open = useUIStore((s) => s.inspectorOpen);
  const tab = useUIStore((s) => s.inspectorTab);
  const activeFile = useUIStore((s) => s.activeFile);
  const setInspectorTab = useUIStore((s) => s.setInspectorTab);
  const toggleInspector = useUIStore((s) => s.toggleInspector);
  const setActiveFile = useUIStore((s) => s.setActiveFile);

  return (
    <InspectorPanel
      open={open}
      tab={tab}
      onTab={setInspectorTab}
      onClose={toggleInspector}
      activeFile={activeFile}
      onSelectFile={setActiveFile}
    />
  );
}

export default definePlugin({
  name: "lyra.builtin.shell-inspector",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("app.inspector", {
      id: "inspector",
      order: 0,
      component: ShellInspector,
    });
  },
});
