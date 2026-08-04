import type { ReactNode } from "react";
import { Icon, type IconName } from "@/ui/icons";
import { useT } from "@/lib/i18n";
import { ButtonPrimitive } from "@/ui/primitives";
import { Tooltip } from "./tooltip";

interface Props {
  icon?: IconName;
  children: ReactNode;
  /** Tooltip label shown on hover — useful when the chip's text is
   *  truncated. Renders via app Tooltip rather than the native title
   *  attribute (200ms snappier, works on focus). */
  title?: string;
  onClose?: () => void;
}

// A compact rounded label used for composer attachments, file refs, etc.
//
// The close affordance (×) is hidden until the chip is hovered/focused —
// keeps a row of chips quiet visually, only surfacing the controls when
// the user reaches for them. Tailwind `group` enables that hover-reveal.
export function Chip({ icon, children, title, onClose }: Props) {
  const t = useT();
  return (
    <Tooltip label={title}>
      <span className="group inline-flex h-[var(--control-height-sm)] items-center gap-1.5 rounded-pill border-[length:var(--control-edge-width)] border-field bg-surface-2 pl-2.5 pr-1 text-ui-sm font-normal text-fg-muted">
        {icon && <Icon name={icon} size="xs" />}
        <span className="max-w-[220px] truncate font-mono">{children}</span>
        {onClose && (
          <ButtonPrimitive
            type="button"
            className="grid h-5 w-5 place-items-center rounded-pill border-0 bg-transparent text-fg-faint opacity-0 scale-[0.96] transition-[opacity,scale,background-color,color] group-hover:opacity-100 group-hover:scale-100 group-focus-within:opacity-100 hover:bg-hover hover:text-fg active:scale-[var(--press-scale)]"
            onClick={onClose}
            aria-label={t("common.remove")}
          >
            <Icon name="x" size="xs" />
          </ButtonPrimitive>
        )}
      </span>
    </Tooltip>
  );
}
