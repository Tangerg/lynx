// Two segmented controls grouped under one Appearance section — both
// pipe a numeric multiplier into uiStore which lights up matching CSS
// vars on `:root`:
//   - Radius scale → `--radius-scale` → every `rounded-*` utility +
//     custom-component border-radii (Tailwind 4 `@theme inline` token
//     bridge does the lift)
//   - Motion scale → `--motion-scale` → CSS `--dur-*` tokens + the
//     motion/react preset durations in lib/motion.ts (live getter on
//     `useUiStore.getState().motionScale`); `motionScale === 0` also
//     sets `data-motion="off"` on :root so Tailwind's literal-ms
//     `duration-*` utilities collapse via a blanket override in
//     globals.css.

import { Segmented, type SegmentedOption } from "@/components/common";
import { useT } from "@/lib/i18n";
import { useUiStore } from "@/state/uiStore";
import { SettingRow } from "./SettingRow";

const RADIUS_OPTIONS: SegmentedOption<number>[] = [
  { value: 0.6, label: "Sharp" },
  { value: 1, label: "Default" },
  { value: 1.4, label: "Soft" },
];

const MOTION_OPTIONS: SegmentedOption<number>[] = [
  { value: 0, label: "Off" },
  { value: 0.6, label: "Fast" },
  { value: 1, label: "Default" },
  { value: 1.5, label: "Slow" },
];

export function ShapeMotionSection() {
  const t = useT();
  const radiusScale = useUiStore((s) => s.radiusScale);
  const motionScale = useUiStore((s) => s.motionScale);
  const setRadiusScale = useUiStore((s) => s.setRadiusScale);
  const setMotionScale = useUiStore((s) => s.setMotionScale);

  return (
    <>
      <SettingRow label={t("settings.radius")} sub={t("settings.radius.sub")} align="start">
        <Segmented
          value={radiusScale}
          options={RADIUS_OPTIONS}
          onChange={setRadiusScale}
          ariaLabel="Corner radius"
        />
      </SettingRow>
      <SettingRow label={t("settings.motion")} sub={t("settings.motion.sub")} align="start">
        <Segmented
          value={motionScale}
          options={MOTION_OPTIONS}
          onChange={setMotionScale}
          ariaLabel="Animation speed"
        />
      </SettingRow>
    </>
  );
}
