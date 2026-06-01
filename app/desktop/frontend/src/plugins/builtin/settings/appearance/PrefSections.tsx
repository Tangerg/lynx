// Small preference rows: message style + streaming style (binary →
// segmented) and UI language (8+ options → dropdown). Bundled into one
// file since each section is < 60 lines.

import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { Icon, Segmented } from "@/components/common";
import { setLocale, useLocale, useT } from "@/lib/i18n";
import { LOCALE, useExtensionPoint } from "@/plugins/sdk";
import { useUiStore } from "@/state/uiStore";
import { SettingRow } from "./SettingRow";

export function MessageStyleSection() {
  const t = useT();
  const messageStyle = useUiStore((s) => s.messageStyle);
  const setMessageStyle = useUiStore((s) => s.setMessageStyle);

  return (
    <SettingRow label={t("settings.messageStyle")} sub={t("settings.messageStyle.sub")}>
      <Segmented
        value={messageStyle}
        options={[
          { value: "bubble", label: t("settings.messageStyle.bubble") },
          { value: "plain", label: t("settings.messageStyle.plain") },
        ]}
        onChange={setMessageStyle}
        ariaLabel={t("settings.messageStyle")}
      />
    </SettingRow>
  );
}

export function StreamRevealSection() {
  const t = useT();
  const streamReveal = useUiStore((s) => s.streamReveal);
  const setStreamReveal = useUiStore((s) => s.setStreamReveal);

  return (
    <SettingRow label={t("settings.streamReveal")} sub={t("settings.streamReveal.sub")}>
      <Segmented
        value={streamReveal}
        options={[
          { value: "smooth", label: t("settings.streamReveal.smooth") },
          { value: "typewriter", label: t("settings.streamReveal.typewriter") },
        ]}
        onChange={setStreamReveal}
        ariaLabel={t("settings.streamReveal")}
      />
    </SettingRow>
  );
}

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
          (8 entries today, more via plugins) doesn't fit a single row.
          Radix DropdownMenu gives keyboard nav + focus management for
          free; visually mirrors Composer's ModePicker so the same
          "trigger looks like a button, content drops below" idiom
          carries between settings + chat. */}
      <DropdownMenu.Root>
        <DropdownMenu.Trigger
          className="inline-flex w-fit min-w-[180px] items-center justify-between gap-2 rounded-md border border-line bg-surface-2 px-3 py-1.5 text-[13px] font-medium text-fg cursor-pointer transition-colors hover:bg-surface-3 data-[state=open]:bg-surface-3 focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-1 focus-visible:outline-accent"
          aria-label={t("settings.language.label")}
        >
          <span>{active.label}</span>
          <Icon name="more" size={11} className="text-fg-faint -rotate-90" />
        </DropdownMenu.Trigger>
        <DropdownMenu.Portal>
          <DropdownMenu.Content
            align="start"
            sideOffset={4}
            className="z-50 min-w-[180px] overflow-hidden rounded-md border border-line-soft bg-surface p-1 shadow-lg animate-rise-in"
          >
            {locales.map((l) => (
              <DropdownMenu.Item
                key={l.id}
                onSelect={() => setLocale(l.id)}
                className="grid cursor-pointer grid-cols-[minmax(0,1fr)_12px] items-center gap-2 rounded-sm px-2.5 py-1.5 text-[12.5px] text-fg-muted outline-none data-[highlighted]:bg-surface-2 data-[highlighted]:text-fg"
              >
                <span className="truncate">{l.label}</span>
                {locale === l.id ? (
                  <Icon name="check" size={12} className="text-accent" />
                ) : (
                  <span aria-hidden />
                )}
              </DropdownMenu.Item>
            ))}
          </DropdownMenu.Content>
        </DropdownMenu.Portal>
      </DropdownMenu.Root>
    </SettingRow>
  );
}
