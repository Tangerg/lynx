// Built-in plugin: the "+" button on the right side of the chat top bar.
// Creates a fresh draft session and opens it (same as ⌘N / the rail "+").
// Plugins are free to register their own top-bar actions alongside.

import { Icon, noDragClasses, Tooltip } from "@/components/common";
import { useCreateSession } from "@/lib/agent/useCreateSession";
import { cn } from "@/lib/utils";
import { definePlugin } from "@/plugins/sdk";

function NewTabButton() {
  // "New session" creates a fresh draft tab — the previous approach of
  // re-opening untabbed sessions silently no-opped when none was free.
  const createSession = useCreateSession();

  return (
    <Tooltip label="New session (⌘N)">
      <button
        type="button"
        onClick={() => void createSession()}
        aria-label="New session"
        className={cn(
          "ml-1 mr-0.5 mb-1 grid h-6.5 w-6.5 shrink-0 place-items-center rounded-md border-0 bg-transparent text-fg-muted hover:bg-surface hover:text-fg",
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
