import { Icon } from "@/ui";
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
    <button
      type="button"
      aria-label={label}
      onClick={onClick}
      tabIndex={visible ? 0 : -1}
      className={cn(
        "absolute bottom-20 left-1/2 -translate-x-1/2 z-[3] grid h-8 w-8 place-items-center rounded-full",
        "bg-surface text-fg border-0",
        "shadow-[var(--shadow-popover)] transition-[opacity,translate,scale,background] duration-[--dur-fast]",
        "hover:bg-surface-2",
        "active:translate-y-0 active:scale-95",
        visible
          ? "opacity-100 translate-y-0 pointer-events-auto"
          : "opacity-0 translate-y-1 pointer-events-none",
      )}
    >
      <Icon name="arrow-down" size={14} />
    </button>
  );
}
