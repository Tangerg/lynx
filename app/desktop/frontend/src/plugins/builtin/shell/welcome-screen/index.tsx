// Built-in plugin: contributes the default welcome screen rendered when
// the conversation has no messages yet.
//
// Lives on the `chat.empty` layout slot so user plugins can replace or
// supplement it (e.g. add a "Recent files" or "What's new" card).

import type { IconName } from "@/components/common";
import type { CommandSpec } from "@/plugins/sdk";
import { Icon } from "@/components/common";
import { useProviders } from "@/lib/data/queries";
import { useT } from "@/lib/i18n";
import { definePlugin, useCommands } from "@/plugins/sdk";
import { comboGlyph } from "@/plugins/builtin/command/comboGlyph";
import { useComposerStore } from "@/state/composerStore";
import { useSessionStore } from "@/state/sessionStore";

interface Suggestion {
  icon: IconName;
  labelKey: string;
  promptKey: string;
}

// Shortcut hints under the suggestions — the command ids whose combo + label
// teach the few keys worth knowing on an empty screen. Read from the registry
// (not hardcoded) so the glyph + label stay in sync with the actual bindings
// and the active locale, and platform-correct (⌘ vs Ctrl) via comboGlyph.
const HINT_COMMAND_IDS = ["command.open", "chat.new", "view.toggle-sidebar"];

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

// First-run onboarding — a keyless user has no model provider configured, so
// the suggestions are dead ends (a send would only hit a provider error). Guide
// them to add an API key first, deep-linking Settings straight to the providers
// pane. The runtime uses API keys (no OAuth, per the §6.2 invariant), so this is
// a "paste your key" step, not a device-code flow.
function SetupCard() {
  const t = useT();
  const onConfigure = () => {
    useSessionStore.getState().setSettingsPane("providers");
    useSessionStore.getState().openMainView({
      id: "settings",
      title: t("settings.title"),
      icon: "settings",
    });
  };
  return (
    <div className="w-full rounded-lg border border-accent/30 bg-[color-mix(in_srgb,var(--color-accent)_8%,transparent)] px-4 py-4 text-left">
      <div className="flex items-start gap-3">
        <Icon name="spark" size={16} className="mt-0.5 shrink-0 text-accent" />
        <div className="flex flex-col items-start gap-2">
          <div className="text-[14px] font-semibold text-fg">{t("welcome.setup.title")}</div>
          <p className="m-0 text-[13.5px] leading-[1.6] text-fg-soft">{t("welcome.setup.sub")}</p>
          <button
            type="button"
            onClick={onConfigure}
            className="mt-0.5 inline-flex items-center gap-2 rounded-md border-0 bg-accent px-3.5 py-2 font-sans text-[13px] font-semibold text-on-accent transition-[filter,transform] duration-150 hover:brightness-110 active:scale-[0.98]"
          >
            <Icon name="settings" size={13} />
            {t("welcome.setup.action")}
          </button>
        </div>
      </div>
    </div>
  );
}

function WelcomeScreen() {
  const t = useT();
  const setValue = useComposerStore((s) => s.setValue);
  const commands = useCommands();
  const hints = HINT_COMMAND_IDS.map((id) => commands.find((c) => c.id === id)).filter(
    (c): c is CommandSpec => !!c?.combo,
  );
  // Keyless = no provider has a saved key. Undefined while the query is in
  // flight — don't flash the setup card before we know (treat as configured
  // until proven otherwise).
  const { data: providers } = useProviders();
  const keyless = providers !== undefined && !providers.some((p) => p.apiKeyMasked !== "");

  return (
    // Centered hero (Codex / ChatGPT empty-state voice). ChatStream renders this
    // in a vertically-centered column directly above the composer, so the
    // positioning lives there — here it's just the centered content group.
    // Sentence-case + period per DESIGN.md §3.
    <div className="mx-auto flex w-full flex-col items-center gap-4 text-center">
      {/* Eyebrow — terminal-style "ready" mark with an accent dot. */}
      <div className="inline-flex items-center gap-2 font-mono text-[11px] text-fg-faint tracking-normal [font-feature-settings:'tnum'] before:content-[''] before:h-1.5 before:w-1.5 before:rounded-full before:bg-accent before:shadow-[0_0_6px_var(--color-accent)]">
        {t("welcome.eyebrow")}
      </div>
      <h1 className="m-0 text-[30px] font-semibold leading-[1.2] tracking-[-0.02em] text-fg">
        {t("welcome.title")}
      </h1>
      <p className="m-0 mb-2 max-w-[440px] text-[14.5px] leading-[1.6] text-fg-muted">
        {t("welcome.sub")}
      </p>
      {keyless ? (
        <SetupCard />
      ) : (
        <>
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
                className="group inline-flex items-center gap-2.5 rounded-lg border border-line bg-surface px-4 py-3.5 font-sans text-[14px] font-medium text-fg-soft text-left transition-[background,border-color,color,transform] duration-150 hover:bg-surface-2 hover:border-line-soft hover:text-fg active:scale-[0.98]"
              >
                <Icon
                  name={s.icon}
                  size={14}
                  className="shrink-0 text-fg-faint group-hover:text-fg"
                />
                <span>{t(s.labelKey)}</span>
              </button>
            ))}
          </div>
          {hints.length > 0 && (
            <div className="mt-3 flex flex-wrap items-center justify-center gap-x-4 gap-y-1.5 font-mono text-[11px] text-fg-faint">
              {hints.map((c) => (
                <span key={c.id} className="inline-flex items-center gap-1.5">
                  <kbd className="rounded border border-line bg-surface-2 px-1.5 py-0.5 text-[10.5px] not-italic text-fg-muted">
                    {comboGlyph(c.combo!)}
                  </kbd>
                  <span>{c.label}</span>
                </span>
              ))}
            </div>
          )}
        </>
      )}
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
