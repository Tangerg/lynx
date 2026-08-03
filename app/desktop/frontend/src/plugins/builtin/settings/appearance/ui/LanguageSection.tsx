// UI language picker — 8+ locales rendered as a dropdown (too many for a
// segmented control). The binary preference rows (message / streaming style)
// live with their only consumer, the Personalization pane.

import { DropdownMenu, Icon, SelectTrigger } from "@/ui";
import { useLocale, useT } from "@/lib/i18n";
import { LOCALE, useExtensionPoint } from "@/plugins/sdk";
import { selectLocale } from "../application/localeSelection";
import { SettingRow } from "../../public";

export function LanguageSection() {
  const t = useT();
  const locale = useLocale();
  const locales = useExtensionPoint(LOCALE);
  const active = locales.find((l) => l.id === locale) ?? locales[0];
  // While locale plugins are still loading (shouldn't happen post
  // PluginProvider, but defensive), the picker would render with no
  // options — bail until at least one is registered.
  if (!active) return null;

  return (
    <SettingRow label={t("settings.language.label")} sub={t("settings.language.sub")}>
      {/* Dropdown rather than segmented because the locale set
          (8 entries today, more via plugins) doesn't fit a single row. */}
      <DropdownMenu.Root>
        <DropdownMenu.Trigger
          render={
            <SelectTrigger
              label={active.label}
              aria-label={t("settings.language.label")}
              className="min-w-[180px]"
            />
          }
        />
        <DropdownMenu.Content align="start" sideOffset={4} className="min-w-[180px]">
          {locales.map((l) => (
            <DropdownMenu.Item
              key={l.id}
              onClick={() => void selectLocale(l)}
              className="grid-cols-[minmax(0,1fr)_12px]"
            >
              <span className="truncate">{l.label}</span>
              {locale === l.id ? (
                <Icon name="check" size="xs" className="text-accent" />
              ) : (
                <span aria-hidden />
              )}
            </DropdownMenu.Item>
          ))}
        </DropdownMenu.Content>
      </DropdownMenu.Root>
    </SettingRow>
  );
}
