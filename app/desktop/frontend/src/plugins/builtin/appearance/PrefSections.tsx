// Two small segmented-control rows: message style (bubble vs plain)
// and UI language. Each section is < 30 lines; bundled together
// instead of two near-empty files.

import type { Locale } from "@/lib/i18n";
import { LOCALES, setLocale, useLocale, useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { useThemeStore } from "@/state/themeStore";

const SEGMENT_BTN_BASE =
  "rounded-sm px-3 py-1 text-[13px] font-medium cursor-pointer transition-colors duration-150";
const SEGMENT_BTN_ACTIVE =
  "bg-surface text-fg shadow-[inset_0_1px_0_rgba(255,255,255,0.03)]";
const SEGMENT_BTN_IDLE = "bg-transparent text-fg-muted hover:text-fg";

export function MessageStyleSection() {
  const t = useT();
  const messageStyle = useThemeStore((s) => s.messageStyle);
  const setMessageStyle = useThemeStore((s) => s.setMessageStyle);

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

  return (
    <div className="grid grid-cols-[140px_1fr] items-center gap-4 py-3">
      <div>
        <div className="text-[15px] font-semibold text-fg">{t("settings.language.label")}</div>
        <div className="mt-0.5 text-[13px] text-fg-muted">{t("settings.language.sub")}</div>
      </div>
      {/* `w-fit` keeps the radiogroup at content width inside the 1fr
          grid cell. Without it, grid items default to justify-self:
          stretch, which overrides inline-flex's shrink-to-content
          behaviour and the segmented control gets dragged across the
          entire row. */}
      <div
        role="radiogroup"
        aria-label={t("settings.language.label")}
        className="inline-flex w-fit gap-1 rounded-md border border-line bg-surface-2 p-1"
      >
        {LOCALES.map((l) => (
          <button
            key={l.id}
            type="button"
            role="radio"
            aria-checked={locale === l.id}
            onClick={() => setLocale(l.id as Locale)}
            className={cn(
              SEGMENT_BTN_BASE,
              locale === l.id ? SEGMENT_BTN_ACTIVE : SEGMENT_BTN_IDLE,
            )}
          >
            {l.label}
          </button>
        ))}
      </div>
    </div>
  );
}
