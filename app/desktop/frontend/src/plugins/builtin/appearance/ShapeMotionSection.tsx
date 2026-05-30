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

import { Slider } from "@/components/common";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";
import { useUiStore } from "@/state/uiStore";

type Option<T> = { value: T; label: string };

const RADIUS_OPTIONS: Option<number>[] = [
  { value: 0.6, label: "Sharp" },
  { value: 1, label: "Default" },
  { value: 1.4, label: "Soft" },
];

const MOTION_OPTIONS: Option<number>[] = [
  { value: 0, label: "Off" },
  { value: 0.6, label: "Fast" },
  { value: 1, label: "Default" },
  { value: 1.5, label: "Slow" },
];

function Segmented<T extends string | number>({
  value,
  options,
  onChange,
  ariaLabel,
}: {
  value: T;
  options: Option<T>[];
  onChange: (v: T) => void;
  ariaLabel: string;
}) {
  return (
    <div
      role="radiogroup"
      aria-label={ariaLabel}
      className="inline-flex w-fit items-center gap-1 rounded-md border border-line bg-surface-2 p-1"
    >
      {options.map((opt) => {
        const active = value === opt.value;
        return (
          <button
            key={String(opt.value)}
            type="button"
            // ARIA Authoring Practices radiogroup pattern — native <input
            // type="radio"> can't be styled as a segmented control.
            // eslint-disable-next-line jsx-a11y/prefer-tag-over-role
            role="radio"
            aria-checked={active}
            onClick={() => onChange(opt.value)}
            className={cn(
              "rounded-sm px-2.5 py-0.5 text-[12px] font-medium cursor-pointer transition-colors duration-150",
              active
                ? "bg-surface text-fg shadow-[inset_0_1px_0_rgba(255,255,255,0.03)]"
                : "bg-transparent text-fg-muted hover:text-fg",
            )}
          >
            {opt.label}
          </button>
        );
      })}
    </div>
  );
}

function Row({ label, sub, children }: { label: string; sub: string; children: React.ReactNode }) {
  return (
    <div className="grid grid-cols-[140px_1fr] items-start gap-4 py-3">
      <div>
        <div className="text-[15px] font-semibold text-fg">{label}</div>
        <div className="mt-0.5 text-[13px] text-fg-muted">{sub}</div>
      </div>
      <div>{children}</div>
    </div>
  );
}

export function ShapeMotionSection() {
  const t = useT();
  const radiusScale = useUiStore((s) => s.radiusScale);
  const motionScale = useUiStore((s) => s.motionScale);
  const contrast = useUiStore((s) => s.contrast);
  const setRadiusScale = useUiStore((s) => s.setRadiusScale);
  const setMotionScale = useUiStore((s) => s.setMotionScale);
  const setContrast = useUiStore((s) => s.setContrast);

  return (
    <>
      <Row
        label={t("settings.radius") || "Corners"}
        sub={
          t("settings.radius.sub") ||
          "Global corner radius. Multiplies every rounded-* token in the app."
        }
      >
        <Segmented
          value={radiusScale}
          options={RADIUS_OPTIONS}
          onChange={setRadiusScale}
          ariaLabel="Corner radius"
        />
      </Row>
      <Row
        label={t("settings.motion") || "Motion"}
        sub={
          t("settings.motion.sub") ||
          "Animation speed. Off skips transitions entirely (matches the OS reduced-motion preference)."
        }
      >
        <Segmented
          value={motionScale}
          options={MOTION_OPTIONS}
          onChange={setMotionScale}
          ariaLabel="Animation speed"
        />
      </Row>
      <Row label={t("settings.contrast")} sub={t("settings.contrast.sub")}>
        <div className="flex items-center gap-3">
          <Slider
            value={contrast}
            min={0}
            max={100}
            onValueChange={setContrast}
            ariaLabel={t("settings.contrast")}
          />
          <span className="w-7 text-right font-mono text-[12px] tabular-nums text-fg-muted">
            {contrast}
          </span>
        </div>
      </Row>
    </>
  );
}
