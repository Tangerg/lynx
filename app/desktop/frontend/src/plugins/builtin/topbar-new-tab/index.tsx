// Built-in plugin: the "+" button on the right side of the chat top bar.
// Picks the next session not already in the tabbar and switches to it.
//
// Used to live as a hardcoded `<button class="tab-new">` inside ChatTopBar;
// pluginifying it means a fork that doesn't want this button can simply
// drop the plugin (and other plugins can register their own top-bar
// actions alongside).

import { Icon } from "@/components/common";
import { useSessions } from "@/lib/queries";
import { definePlugin } from "@/plugins/sdk";
import { useSessionStore } from "@/state/sessionStore";

function NewTabButton() {
  const { data: sessions = [] } = useSessions();
  const tabIds = useSessionStore((s) => s.tabIds);
  const selectTab = useSessionStore((s) => s.selectTab);

  const onClick = () => {
    const candidate = sessions.find((s) => !tabIds.includes(s.id));
    if (candidate) selectTab(candidate.id);
  };

  return (
    <button
      type="button"
      onClick={onClick}
      title="New session (⌘N)" aria-label="New session (⌘N)"
      className="ml-1 mr-0.5 mb-1 grid h-6.5 w-6.5 shrink-0 place-items-center rounded-md border-0 bg-transparent text-fg-muted cursor-pointer [-webkit-app-region:no-drag] [--wails-draggable:no-drag] hover:bg-surface hover:text-fg"
    >
      <Icon name="plus" size={13} />
    </button>
  );
}

export default definePlugin({
  name: "lyra.builtin.topbar-new-tab",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("chat.topbar.actions", {
      id: "new-tab",
      order: 0,
      component: NewTabButton,
    });
  },
});
