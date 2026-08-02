import { Checkbox, Segmented } from "@/ui";
import { useT } from "@/lib/i18n";
import {
  useCompletionSoundPreference,
  useStreamRevealPreference,
} from "../application/personalizationPreferences";
import { SettingRow } from "../../public";

export function CompletionSoundSection() {
  const t = useT();
  const { completionSound, setCompletionSound } = useCompletionSoundPreference();

  return (
    <SettingRow label={t("settings.completionSound")} sub={t("settings.completionSound.sub")}>
      <Checkbox
        checked={completionSound}
        onCheckedChange={setCompletionSound}
        label={t("settings.completionSound.toggle")}
      />
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
