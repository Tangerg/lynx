import { PROVIDERS_PANE } from "@/plugins/builtin/settings/public/panes";
import type { IconName } from "@/ui";
import { Button, Icon, Kbd, PillButton, Surface, Tooltip } from "@/ui";
import { comboGlyph } from "@/lib/combo";
import { useProviders } from "@/plugins/builtin/settings/providers/public/queries";
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
                <Button
                  variant="soft"
                  className="gap-2"
                  onClick={() => setValue(t(suggestion.promptKey))}
                >
                  <Icon
                    name={suggestion.icon as IconName}
                    size="sm"
                    className="shrink-0 text-fg-muted"
                  />
                  <span>{t(suggestion.labelKey)}</span>
                </Button>
              </Tooltip>
            ))}
          </div>
          {hints.length > 0 && (
            <div className="mt-4 flex flex-wrap items-center justify-center gap-x-4 gap-y-1.5 font-mono text-ui-sm text-fg-faint">
              {hints.map((command) => (
                <span key={command.id} className="inline-flex items-center gap-1.5">
                  <Kbd>{comboGlyph(command.combo!)}</Kbd>
                  <span>{t(command.label)}</span>
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
    openWorkspaceSettingsPane(PROVIDERS_PANE);
  };

  return (
    <Surface className="w-full text-left">
      <div className="flex items-start gap-3">
        <Icon name="spark" size="md" className="mt-0.5 shrink-0 text-accent" />
        <div className="flex flex-col items-start gap-2">
          <div className="text-balance text-ui-lg font-semibold text-fg">
            {t("welcome.setup.title")}
          </div>
          <p className="m-0 text-pretty text-ui-lg leading-relaxed text-fg-soft">
            {t("welcome.setup.sub")}
          </p>
          <PillButton variant="solid" onClick={onConfigure} className="mt-0.5 font-semibold">
            <Icon name="settings" size="sm" />
            {t("welcome.setup.action")}
          </PillButton>
        </div>
      </div>
    </Surface>
  );
}
