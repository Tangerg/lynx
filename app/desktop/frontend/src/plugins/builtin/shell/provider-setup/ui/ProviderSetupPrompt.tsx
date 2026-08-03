import { PROVIDERS_PANE } from "@/plugins/builtin/settings/public/panes";
import { Icon, PillButton, Surface } from "@/ui";
import { useT } from "@/lib/i18n";
import { openWorkspaceSettingsPane } from "@/plugins/builtin/workspace/public/navigation";
import {
  needsProviderSetup,
  useProviders,
} from "@/plugins/builtin/settings/providers/public/queries";

/**
 * The one thing that may still appear on an otherwise empty home: an install with no
 * key anywhere cannot send a message, so the way to fix that has to be on the screen
 * you land on. Renders nothing in the normal case, which is what keeps the home to a
 * title and an input.
 *
 * It lives on the SHELL side and asks the providers context its question through that
 * context's public surface, rather than living inside providers and reaching back out
 * to the workspace to open a pane — that direction closes a cycle (workspace already
 * consumes provider queries) and `check-builtin-contexts` says so. Consumer →
 * provider's public API is the direction that composes.
 */
export function ProviderSetupPrompt() {
  const t = useT();
  const { data: providers } = useProviders();
  if (!needsProviderSetup(providers)) return null;

  return (
    <Surface className="w-full text-left">
      <div className="flex items-start gap-3">
        <Icon name="spark" size="md" className="mt-0.5 shrink-0 text-accent" />
        <div className="flex flex-col items-start gap-2">
          <div className="text-balance text-ui-md font-semibold text-fg">
            {t("providers.setup.title")}
          </div>
          <p className="m-0 text-pretty text-ui-md leading-prose text-fg-soft">
            {t("providers.setup.sub")}
          </p>
          <PillButton
            variant="solid"
            onClick={() => openWorkspaceSettingsPane(PROVIDERS_PANE)}
            className="mt-0.5 font-semibold"
          >
            <Icon name="settings" size="sm" />
            {t("providers.setup.action")}
          </PillButton>
        </div>
      </div>
    </Surface>
  );
}
