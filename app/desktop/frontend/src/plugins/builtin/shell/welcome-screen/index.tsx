// Built-in plugin: contributes the default welcome screen rendered when
// the conversation has no messages yet.
//
// Lives on the `chat.empty` layout slot so user plugins can replace or
// supplement it (e.g. add a "Recent files" or "What's new" card).

import type { IconName } from "@/components/common";
import { Icon } from "@/components/common";
import { useT } from "@/lib/i18n";
import { definePlugin } from "@/plugins/sdk";
import { useComposerStore } from "@/state/composerStore";

interface Suggestion {
  icon: IconName;
  labelKey: string;
  promptKey: string;
}

// Suggestions are an array of (icon + i18n key pair) so labels + prompts
// switch with the active locale. Keep order intentional (recognition
// order: refactor → search → review → checklist).
const SUGGESTIONS: Suggestion[] = [
  {
    icon: "spark",
    labelKey: "welcome.suggest.refactor",
    promptKey: "welcome.suggest.refactor.prompt",
  },
  {
    icon: "search",
    labelKey: "welcome.suggest.search",
    promptKey: "welcome.suggest.search.prompt",
  },
  {
    icon: "branch",
    labelKey: "welcome.suggest.review",
    promptKey: "welcome.suggest.review.prompt",
  },
  {
    icon: "list",
    labelKey: "welcome.suggest.checklist",
    promptKey: "welcome.suggest.checklist.prompt",
  },
];

function WelcomeScreen() {
  const t = useT();
  const setValue = useComposerStore((s) => s.setValue);

  return (
    // Centered to chat-measure (760px) so when the first message lands
    // the prose sits in the same column. Sentence-case + period per
    // DESIGN.md §3 (Vercel headlines voice).
    <div className="mx-auto flex max-w-[760px] flex-col items-start gap-3.5 px-6 pt-20 pb-60">
      {/* Eyebrow — terminal-style "ready" mark with an accent dot. */}
      <div className="mb-1.5 inline-flex items-center gap-2 font-mono text-[11px] text-fg-faint tracking-normal [font-feature-settings:'tnum'] before:content-[''] before:h-1.5 before:w-1.5 before:rounded-full before:bg-accent before:shadow-[0_0_6px_var(--color-accent)]">
        {t("welcome.eyebrow")}
      </div>
      <h1 className="m-0 text-[40px] font-semibold leading-[1.15] tracking-[-0.02em] text-fg">
        {t("welcome.title")}
      </h1>
      <p className="m-0 mb-4 max-w-[600px] text-[15px] leading-[1.65] text-fg-soft">
        {t("welcome.sub")}
      </p>
      <div className="grid w-full grid-cols-2 gap-2">
        {SUGGESTIONS.map((s) => (
          <button
            key={s.labelKey}
            type="button"
            onClick={() => setValue(t(s.promptKey))}
            // Native tooltip shows the actual prompt prefix that lands in
            // the composer, so the user can preview what the suggestion
            // will do (the visible label is intentionally short).
            title={t(s.promptKey)}
            className="group inline-flex items-center gap-2.5 rounded-md border border-line bg-surface px-3.5 py-3 font-sans text-[14px] font-medium text-fg-soft text-left cursor-pointer transition-[background,border-color,color,transform] duration-150 hover:bg-surface-2 hover:border-line-soft hover:text-fg active:scale-[0.98]"
          >
            <Icon name={s.icon} size={14} className="shrink-0 text-fg-faint group-hover:text-fg" />
            <span>{t(s.labelKey)}</span>
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
