// Built-in plugin: bottom of the collapsed sidebar — tools/MCP, settings,
// user avatar. Uses a flex spacer to push everything else off the bottom.

import { Icon, IconButton } from "@/components/common";
import { definePlugin } from "@/plugins/sdk";

function Spacer()  { return <div style={{ flex: 1 }} />; }
function ToolsBtn() {
  return <IconButton variant="rail" title="Tools / MCP"><Icon name="tool" size={16} /></IconButton>;
}
function SettingsBtn() {
  return <IconButton variant="rail" title="Settings"><Icon name="settings" size={16} /></IconButton>;
}
function UserAvatar() {
  return <div className="rail-user" title="You · jdoe@longbridge-inc.com">J</div>;
}

export default definePlugin({
  name: "lyra.builtin.sidebar-rail-bottom",
  version: "1.0.0",
  setup({ host }) {
    host.sidebar.registerRailItem({ id: "rail-spacer",   order: 800, component: Spacer });
    host.sidebar.registerRailItem({ id: "rail-tools",    order: 900, component: ToolsBtn });
    host.sidebar.registerRailItem({ id: "rail-settings", order: 910, component: SettingsBtn });
    host.sidebar.registerRailItem({ id: "rail-user",     order: 920, component: UserAvatar });
  },
});
