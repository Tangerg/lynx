import { Icon, Tooltip } from "@/components/common";
import { useT } from "@/lib/i18n";
import { cn } from "@/lib/utils";

interface Props {
  visible: boolean;
  onClick: () => void;
}

// Floating "scroll to bottom" affordance. Renders inside the composer
// area (positioned above the composer card) so it never overlaps the
// last message, and stays out of the layout flow.
//
// Animates in/out via opacity + translateY rather than mount/unmount,
// so the user gets a soft reveal instead of a pop-in. When `visible`
// is false it's still in the DOM but pointer-events: none + opacity: 0.
export function JumpToBottomButton({ visible, onClick }: Props) {
  const t = useT();
  const label = t("chat.jumpToBottom");
  return (
    <Tooltip label={visible ? label : ""}>
      <button
        type="button"
        aria-label={label}
        onClick={onClick}
        tabIndex={visible ? 0 : -1}
        className={cn(
          "absolute bottom-24 right-7 z-[3] grid h-9 w-9 place-items-center rounded-md",
          "bg-surface text-fg border border-line/40",
          "shadow-middle transition-[opacity,transform,background-color,border-color] duration-150 ease-out",
          "hover:bg-surface-2 hover:border-line-soft",
          "active:translate-y-0 active:scale-95",
          visible
            ? "opacity-100 translate-y-0 pointer-events-auto"
            : "opacity-0 translate-y-2 pointer-events-none",
        )}
      >
        <Icon name="arrow-down" size={14} />
      </button>
    </Tooltip>
  );
}
