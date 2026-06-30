// Personalization preference rows — message style + streaming reveal style +
// completion sound. Binary picks rendered as a Segmented / Checkbox control.
// Live here (not in the Appearance plugin) because the Personalization pane is
// their only consumer; the shared `SettingRow` primitive sits at the
// settings-domain root.

import { useId } from "react";
import { Checkbox, Segmented } from "@/components/common";
import { useT } from "@/lib/i18n";
import { useUiStore } from "@/state/uiStore";
import { SettingRow } from "../SettingRow";

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

export function CompletionSoundSection() {
  const t = useT();
  const completionSound = useUiStore((s) => s.completionSound);
  const setCompletionSound = useUiStore((s) => s.setCompletionSound);
  const id = useId();

  return (
    <SettingRow label={t("settings.completionSound")} sub={t("settings.completionSound.sub")}>
      <label htmlFor={id} className="inline-flex items-center gap-1.5 text-[12.5px] text-fg-muted">
        <Checkbox
          id={id}
          checked={completionSound}
          onCheckedChange={setCompletionSound}
          ariaLabel={t("settings.completionSound")}
        />
        <span>{t("settings.completionSound.toggle")}</span>
      </label>
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
