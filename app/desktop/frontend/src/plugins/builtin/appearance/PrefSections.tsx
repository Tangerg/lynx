// Two small preference rows: message style (binary → segmented) and
// UI language (8+ options → dropdown). Bundled into one file since
// each section is < 60 lines.

import * as DropdownMenu from "@radix-ui/react-dropdown-menu";
import { Icon } from "@/components/common";
import { setLocale, useLocale, useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { useLocales } from "@/plugins/sdk";
import { useUiStore } from "@/state/uiStore";

const SEGMENT_BTN_BASE =
  "rounded-sm px-3 py-1 text-[13px] font-medium cursor-pointer transition-colors duration-150";
const SEGMENT_BTN_ACTIVE = "bg-surface text-fg shadow-[inset_0_1px_0_rgba(255,255,255,0.03)]";
const SEGMENT_BTN_IDLE = "bg-transparent text-fg-muted hover:text-fg";

export function MessageStyleSection() {
  const t = useT();
  const messageStyle = useUiStore((s) => s.messageStyle);
  const setMessageStyle = useUiStore((s) => s.setMessageStyle);

  return (
    <div className="grid grid-cols-[140px_1fr] items-center gap-4 py-3">
      <div>
        <div className="text-[15px] font-semibold text-fg">{t("settings.messageStyle")}</div>
        <div className="mt-0.5 text-[13px] text-fg-muted">{t("settings.messageStyle.sub")}</div>
      </div>
      <div
        role="radiogroup"
        aria-label={t("settings.messageStyle")}
        className="inline-flex w-fit gap-1 rounded-md border border-line bg-surface-2 p-1"
      >
        {(["bubble", "plain"] as const).map((s) => (
          <button
            key={s}
            type="button"
            role="radio"
            aria-checked={messageStyle === s}
            onClick={() => setMessageStyle(s)}
            className={cn(
              SEGMENT_BTN_BASE,
              messageStyle === s ? SEGMENT_BTN_ACTIVE : SEGMENT_BTN_IDLE,
            )}
          >
            {t(`settings.messageStyle.${s}`)}
          </button>
        ))}
      </div>
    </div>
  );
}

export function LanguageSection() {
  const t = useT();
  const locale = useLocale();
  const locales = useLocales();
  const active = locales.find((l) => l.id === locale) ?? locales[0];
  // While locale plugins are still loading (shouldn't happen post
  // PluginProvider, but defensive), the picker would render with no
  // options — bail until at least one is registered.
  if (!active) return null;

  return (
    <div className="grid grid-cols-[140px_1fr] items-center gap-4 py-3">
      <div>
        <div className="text-[15px] font-semibold text-fg">{t("settings.language.label")}</div>
        <div className="mt-0.5 text-[13px] text-fg-muted">{t("settings.language.sub")}</div>
      </div>
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
    </div>
  );
}
