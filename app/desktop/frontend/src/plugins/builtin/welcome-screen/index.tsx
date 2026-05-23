// Built-in plugin: contributes the default welcome screen rendered when
// the conversation has no messages yet.
//
// Lives on the `chat.empty` layout slot so user plugins can replace or
// supplement it (e.g. add a "Recent files" or "What's new" card).

import { Icon, type IconName } from "@/components/common";
import { definePlugin } from "@/plugins/sdk";
import { useComposerStore } from "@/state/composerStore";

type Suggestion = { icon: IconName; label: string; prompt: string };

const SUGGESTIONS: Suggestion[] = [
  { icon: "spark",  label: "Plan a refactor",         prompt: "Help me plan a refactor of " },
  { icon: "search", label: "Search the codebase",     prompt: "Search the codebase for " },
  { icon: "branch", label: "Review recent changes",   prompt: "Review my recent changes on " },
  { icon: "list",   label: "Draft a checklist",       prompt: "Draft a checklist for " },
];

function WelcomeScreen() {
  const setValue = useComposerStore((s) => s.setValue);

  return (
    <div className="welcome">
      <div className="welcome-eyebrow">agent ready</div>
      <h1 className="welcome-title">What can I help you build today.</h1>
      <p className="welcome-sub">
        Start a conversation, paste a stack trace, or pick a suggestion below.
      </p>
      <div className="welcome-grid">
        {SUGGESTIONS.map((s) => (
          <button
            key={s.label}
            className="welcome-card"
            onClick={() => setValue(s.prompt)}
            type="button"
          >
            <Icon name={s.icon} size={14} />
            <span>{s.label}</span>
          </button>
        ))}
      </div>
    </div>
  );
}

export default definePlugin({
  name: "lyra.builtin.welcome-screen",
  version: "1.0.0",
  setup({ host }) {
    host.layout.register("chat.empty", {
      id: "welcome",
      order: 0,
      component: WelcomeScreen,
    });
  },
});
