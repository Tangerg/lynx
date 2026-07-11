import type { IconName } from "@/ui";
import { Icon, Kbd, Tooltip } from "@/ui";
import { comboGlyph } from "@/lib/combo";
import { useProviders } from "@/plugins/builtin/settings/providers/public/data";
import { useT } from "@/lib/i18n";
import { useSetComposerText } from "@/plugins/builtin/chat/composer/public/draft";
import { openWorkspaceSettingsPane } from "@/plugins/builtin/workspace/public/navigation";
import { useCommands } from "@/plugins/sdk";
import {
  needsProviderSetup,
  WELCOME_SUGGESTIONS,
  welcomeHintCommands,
} from "../application/welcomeContent";

export function WelcomeScreen() {
  const t = useT();
  const setValue = useSetComposerText();
  const hints = welcomeHintCommands(useCommands());
  const { data: providers } = useProviders();

  return (
    <div className="mx-auto w-full max-w-[var(--content-max)]">
      {needsProviderSetup(providers) ? (
        <SetupCard />
      ) : (
        <>
          <div className="flex flex-wrap justify-center gap-2">
            {WELCOME_SUGGESTIONS.map((suggestion) => (
              <Tooltip key={suggestion.labelKey} label={t(suggestion.promptKey)} side="bottom">
                <button
                  type="button"
                  onClick={() => setValue(t(suggestion.promptKey))}
                  className="inline-flex h-8 items-center gap-2 rounded-[12px] border-0 bg-surface px-3 text-[12.5px] font-medium text-fg-soft transition-[background-color,color,transform] duration-[120ms] hover:bg-surface-2 hover:text-fg active:scale-[0.98]"
                >
                  <Icon
                    name={suggestion.icon as IconName}
                    size={14}
                    className="shrink-0 text-fg-muted"
                  />
                  <span>{t(suggestion.labelKey)}</span>
                </button>
              </Tooltip>
            ))}
          </div>
          {hints.length > 0 && (
            <div className="mt-4 flex flex-wrap items-center justify-center gap-x-4 gap-y-1.5 font-mono text-[11px] text-fg-faint">
              {hints.map((command) => (
                <span key={command.id} className="inline-flex items-center gap-1.5">
                  <Kbd>{comboGlyph(command.combo!)}</Kbd>
                  <span>{command.label}</span>
                </span>
              ))}
            </div>
          )}
        </>
      )}
    </div>
  );
}

function SetupCard() {
  const t = useT();
  const onConfigure = () => {
    openWorkspaceSettingsPane("providers");
  };

  return (
    <div className="w-full rounded-[12px] bg-surface px-4 py-4 text-left">
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
