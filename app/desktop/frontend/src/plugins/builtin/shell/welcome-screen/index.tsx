// Built-in plugin: contributes the default welcome screen rendered when
// the conversation has no messages yet.
//
// Lives on the `chat.empty` layout slot so user plugins can replace or
// supplement it (e.g. add a "Recent files" or "What's new" card).

import type { CommandSpec } from "@/plugins/sdk";
import type { IconName } from "@/ui";
import { AgentSurface } from "@/ui/agent";
import { Icon, Tooltip } from "@/ui";
import { useProviders } from "@/lib/data/queries";
import { useT } from "@/lib/i18n";
import { definePlugin, useCommands } from "@/plugins/sdk";
import { comboGlyph } from "@/lib/combo";
import { useSetComposerText } from "@/plugins/builtin/chat/composer/public/draft";
import { openWorkspaceSettingsPane } from "@/plugins/builtin/workspace/public/navigation";

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
    openWorkspaceSettingsPane("providers", t("settings.title"));
  };
  return (
    <div className="w-full rounded-lg bg-surface px-4 py-4 text-left">
      <div className="flex items-start gap-3">
        <Icon name="spark" size={16} className="mt-0.5 shrink-0 text-accent" />
        <div className="flex flex-col items-start gap-2">
          <div className="text-balance text-[14px] font-semibold text-fg">
            {t("welcome.setup.title")}
          </div>
          <p className="m-0 text-pretty text-[13.5px] leading-[1.6] text-fg-soft">
            {t("welcome.setup.sub")}
          </p>
          <button
            type="button"
            onClick={onConfigure}
            className="mt-0.5 inline-flex items-center gap-2 rounded-full border-0 bg-fg px-3.5 py-2 font-sans text-[13px] font-semibold text-on-fg transition-[filter,transform] duration-150 hover:brightness-110 active:scale-[0.96]"
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
  const setValue = useSetComposerText();
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
    <div className="mx-auto w-full max-w-[760px]">
      {keyless ? (
        <SetupCard />
      ) : (
        <>
          <div className="grid w-full grid-cols-3 gap-3">
            {SUGGESTIONS.slice(0, 3).map((s) => (
              <Tooltip key={s.labelKey} label={t(s.promptKey)} side="bottom">
                <button
                  type="button"
                  onClick={() => setValue(t(s.promptKey))}
                  className="group min-h-[76px] rounded-[12px] border-0 bg-transparent p-0 text-left transition-transform duration-150 active:scale-[0.99]"
                >
                  <AgentSurface className="h-full px-4 py-3 transition-colors group-hover:bg-surface-2">
                    <Icon name={s.icon} size={16} className="mb-3 shrink-0 text-fg-muted" />
                    <div className="truncate text-[13px] font-semibold leading-[18px] text-fg">
                      {t(s.labelKey)}
                    </div>
                    <div className="mt-1 truncate text-[12.5px] leading-[17px] text-fg-muted">
                      {t(s.promptKey)}
                    </div>
                  </AgentSurface>
                </button>
              </Tooltip>
            ))}
          </div>
          {hints.length > 0 && (
            <div className="mt-4 flex flex-wrap items-center justify-center gap-x-4 gap-y-1.5 font-mono text-[11px] text-fg-faint">
              {hints.map((c) => (
                <span key={c.id} className="inline-flex items-center gap-1.5">
                  <kbd className="rounded border-[0.5px] border-field bg-surface-2 px-1.5 py-0.5 text-[10.5px] not-italic text-fg-muted">
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
