// Personalization preference rows — message style + streaming reveal style.
// Both are binary picks rendered as a Segmented control. Live here (not in the
// Appearance plugin) because the Personalization pane is their only consumer;
// the shared `SettingRow` primitive sits at the settings-domain root.

import { Segmented } from "@/components/common";
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
