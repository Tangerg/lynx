// Built-in plugin: the "+" button on the right side of the chat top bar.
// Creates a fresh draft session and opens it (same as ⌘N / the rail "+").
//
// Used to live as a hardcoded `<button class="tab-new">` inside PanelTabBar;
// pluginifying it means a fork that doesn't want this button can simply
// drop the plugin (and other plugins can register their own top-bar
// actions alongside).

import { Icon, noDragClasses, Tooltip } from "@/components/common";
import { useCreateSession } from "@/lib/agent/useCreateSession";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";

function NewTabButton() {
  // "New session" must CREATE one (a hidden draft, opened active) — it used to
  // just open an existing untabbed session and silently no-op when none was
  // free, which read as a dead button.
  const createSession = useCreateSession();

  return (
    <Tooltip label="New session (⌘N)">
      <button
        type="button"
        onClick={() => void createSession()}
        aria-label="New session"
        className={cn(
          "ml-1 mr-0.5 mb-1 grid h-6.5 w-6.5 shrink-0 place-items-center rounded-md border-0 bg-transparent text-fg-muted cursor-pointer hover:bg-surface hover:text-fg",
          noDragClasses,
        )}
      >
        <Icon name="plus" size={13} />
      </button>
    </Tooltip>
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
