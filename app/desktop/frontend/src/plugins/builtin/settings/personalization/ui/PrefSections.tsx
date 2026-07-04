import { useId } from "react";
import { Checkbox, Segmented } from "@/ui";
import { useT } from "@/lib/i18n";
import {
  useCompletionSoundPreference,
  useMessageStylePreference,
  useStreamRevealPreference,
} from "../application/personalizationPreferences";
import { SettingRow } from "../../public";

export function MessageStyleSection() {
  const t = useT();
  const { messageStyle, setMessageStyle } = useMessageStylePreference();

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
  const { completionSound, setCompletionSound } = useCompletionSoundPreference();
  const id = useId();

  return (
    <SettingRow label={t("settings.completionSound")} sub={t("settings.completionSound.sub")}>
      <label htmlFor={id} className="inline-flex items-center gap-2 text-[13px] text-fg-muted">
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
  const { streamReveal, setStreamReveal } = useStreamRevealPreference();

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
