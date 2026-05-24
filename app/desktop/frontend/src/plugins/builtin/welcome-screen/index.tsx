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
    // Centered to chat-measure (760px) so when the first message lands
    // the prose sits in the same column. Sentence-case + period per
    // DESIGN.md §3 (Vercel headlines voice).
    <div className="mx-auto flex max-w-[760px] flex-col items-start gap-3.5 px-6 pt-20 pb-60">
      {/* Eyebrow — terminal-style "ready" mark with an accent dot. */}
      <div className="mb-1.5 inline-flex items-center gap-2 font-mono text-[10.5px] text-fg-faint tracking-normal [font-feature-settings:'tnum'] before:content-[''] before:h-1.5 before:w-1.5 before:rounded-full before:bg-accent before:shadow-[0_0_6px_var(--color-accent)]">
        agent ready
      </div>
      <h1 className="m-0 text-[40px] font-semibold leading-[1.15] tracking-[-0.02em] text-fg">
        What can I help you build today.
      </h1>
      <p className="m-0 mb-4 max-w-[600px] text-[14px] leading-[1.65] text-fg-soft">
        Start a conversation, paste a stack trace, or pick a suggestion below.
      </p>
      <div className="grid w-full grid-cols-2 gap-2">
        {SUGGESTIONS.map((s) => (
          <button
            key={s.label}
            type="button"
            onClick={() => setValue(s.prompt)}
            className="group inline-flex items-center gap-2.5 rounded-md border border-line bg-surface px-3.5 py-3 font-sans text-[13px] font-medium text-fg-soft text-left cursor-pointer transition-[background,border-color,color,transform] duration-150 hover:bg-surface-2 hover:border-line-soft hover:text-fg active:scale-[0.98]"
          >
            <Icon name={s.icon} size={14} className="shrink-0 text-fg-faint group-hover:text-fg" />
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
