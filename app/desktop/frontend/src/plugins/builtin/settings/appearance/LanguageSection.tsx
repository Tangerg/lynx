// UI language picker — 8+ locales rendered as a dropdown (too many for a
// segmented control). The binary preference rows (message / streaming style)
// live with their only consumer, the Personalization pane.

import {
  Icon,
  Menu as BaseMenu,
  MENU_CONTENT_CLASSES,
  MENU_ITEM_CLASSES,
} from "@/components/common";
import { setLocale, useLocale, useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { LOCALE, useExtensionPoint } from "@/plugins/sdk";
import { SettingRow } from "../SettingRow";

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
      <BaseMenu.Root>
        <BaseMenu.Trigger
          className="inline-flex w-fit min-w-[180px] items-center justify-between gap-2 rounded-md border border-line bg-surface-2 px-3 py-1.5 text-[13px] font-medium text-fg transition-colors hover:bg-surface-3 data-[popup-open]:bg-surface-3 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent"
          aria-label={t("settings.language.label")}
        >
          <span>{active.label}</span>
          <Icon name="more" size={11} className="text-fg-faint -rotate-90" />
        </BaseMenu.Trigger>
        <BaseMenu.Portal>
          <BaseMenu.Positioner align="start" sideOffset={4}>
            <BaseMenu.Popup className={cn(MENU_CONTENT_CLASSES, "min-w-[180px]")}>
              {locales.map((l) => (
                <BaseMenu.Item
                  key={l.id}
                  onClick={() => setLocale(l.id)}
                  className={cn(MENU_ITEM_CLASSES, "grid-cols-[minmax(0,1fr)_12px]")}
                >
                  <span className="truncate">{l.label}</span>
                  {locale === l.id ? (
                    <Icon name="check" size={12} className="text-accent" />
                  ) : (
                    <span aria-hidden />
                  )}
                </BaseMenu.Item>
              ))}
            </BaseMenu.Popup>
          </BaseMenu.Positioner>
        </BaseMenu.Portal>
      </BaseMenu.Root>
    </SettingRow>
  );
}
