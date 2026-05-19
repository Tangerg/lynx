// Built-in plugin: contributes the "Lyra" brand mark into both sidebar
// brand slots. The expanded slot renders icon + product name; the rail
// (collapsed) slot is icon-only.
//
// A re-skinned build can replace this single plugin to swap branding
// everywhere — no fork of SidebarExpanded / SidebarRail required.

import { Icon } from "@/components/common";
import { definePlugin } from "@/plugins/sdk";

function FullBrand() {
  return (
    <>
      <div className="brand-mark"><Icon name="spark" size={16} /></div>
      <div>
        <div className="brand-name">Lyra</div>
      </div>
    </>
  );
}

function RailBrand() {
  return <div className="brand-mark"><Icon name="spark" size={16} /></div>;
}

export default definePlugin({
  name: "lyra.builtin.sidebar-brand",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("sidebar.brand", {
      id: "brand", order: 0, component: FullBrand,
    });
    host.layout.register("sidebar.rail.brand", {
      id: "rail-brand", order: 0, component: RailBrand,
    });
  },
});
